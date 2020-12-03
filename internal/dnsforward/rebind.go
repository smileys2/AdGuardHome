// DNS Rebinding protection

package dnsforward

import (
	"fmt"
	"net"
	"strings"

	"github.com/AdguardTeam/AdGuardHome/internal/dnsfilter"
	"github.com/AdguardTeam/golibs/log"
	"github.com/miekg/dns"
)

type dnsRebindChecker struct {
}

// IsPrivate reports whether ip is a private address, according to
// RFC 1918 (IPv4 addresses) and RFC 4193 (IPv6 addresses).
func (*dnsRebindChecker) isPrivate(ip net.IP) bool {
	//TODO: remove once https://github.com/golang/go/pull/42793 makes it to stdlib
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc
}

func (c *dnsRebindChecker) isRebindHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return c.isRebindIP(ip)
	}

	return host == "localhost"
}

func (c *dnsRebindChecker) isRebindIP(ip net.IP) bool {
	// This is compatible with dnsmasq definition
	// See: https://github.com/imp/dnsmasq/blob/4e7694d7107d2299f4aaededf8917fceb5dfb924/src/rfc1035.c#L412

	rebind := false
	if ip4 := ip.To4(); ip4 != nil {

		/* 0.0.0.0/8 (RFC 5735 section 3. "here" network) */
		rebind = ip4[0] == 0 ||

			/* 10.0.0.0/8     (private)  */
			ip4[0] == 10 ||

			/* 172.16.0.0/12  (private)  */
			(ip4[0] == 172 && ip4[1]&0x10 == 0x10) ||

			/* 169.254.0.0/16 (zeroconf) */
			(ip4[0] == 169 && ip4[1] == 254) ||

			/* 192.0.2.0/24   (test-net) */
			(ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) ||

			/* 198.51.100.0/24(test-net) */
			(ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) ||

			/* 203.0.113.0/24 (test-net) */
			(ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) ||

			/* 255.255.255.255/32 (broadcast)*/
			ip4.Equal(net.IPv4bcast)
	} else {
		rebind = ip.Equal(net.IPv6zero) || ip.Equal(net.IPv6unspecified) ||
			ip.Equal(net.IPv6interfacelocalallnodes) ||
			ip.Equal(net.IPv6linklocalallnodes) ||
			ip.Equal(net.IPv6linklocalallrouters)
	}

	return rebind || c.isPrivate(ip) || ip.IsLoopback()
}

// Checks DNS rebinding attacks
// Note both whitelisted and cached hosts will bypass rebinding check (see: processFilteringAfterResponse()).
func (s *Server) isResponseRebind(domain, host string) bool {
	if !s.conf.RebindingProtectionEnabled {
		return false
	}

	if log.GetLevel() >= log.DEBUG {
		timer := log.StartTimer()
		defer timer.LogElapsed("DNS Rebinding check for %s -> %s", domain, host)
	}

	for _, h := range s.conf.RebindingAllowedHosts {
		if strings.HasSuffix(domain, h) {
			return false
		}
	}

	c := dnsRebindChecker{}
	return c.isRebindHost(host)
}

func processRebindingFilteringAfterResponse(ctx *dnsContext) int {
	s := ctx.srv
	d := ctx.proxyCtx
	res := ctx.result
	var err error

	if !ctx.responseFromUpstream || res.Reason == dnsfilter.ReasonRewrite {
		return resultDone
	}

	originalRes := d.Res
	ctx.result, err = s.preventRebindResponse(ctx)
	if err != nil {
		ctx.err = err
		return resultError
	}
	if ctx.result != nil {
		ctx.origResp = originalRes // matched by response
	} else {
		ctx.result = &dnsfilter.Result{}
	}

	return resultDone
}

func (s *Server) preventRebindResponse(ctx *dnsContext) (*dnsfilter.Result, error) {
	d := ctx.proxyCtx

	for _, a := range d.Res.Answer {
		m := ""
		domainName := ""
		host := ""

		switch v := a.(type) {
		case *dns.CNAME:
			host = strings.TrimSuffix(v.Target, ".")
			domainName = v.Hdr.Name
			m = fmt.Sprintf("DNSRebind: Checking CNAME %s for %s", v.Target, v.Hdr.Name)

		case *dns.A:
			host = v.A.String()
			domainName = v.Hdr.Name
			m = fmt.Sprintf("DNSRebind: Checking record A (%s) for %s", host, v.Hdr.Name)

		case *dns.AAAA:
			host = v.AAAA.String()
			domainName = v.Hdr.Name
			m = fmt.Sprintf("DNSRebind: Checking record AAAA (%s) for %s", host, v.Hdr.Name)

		default:
			continue
		}

		s.RLock()
		if !s.conf.RebindingProtectionEnabled {
			s.RUnlock()
			continue
		}

		log.Debug(m)
		blocked := s.isResponseRebind(domainName, host)
		s.RUnlock()

		if blocked {
			res := &dnsfilter.Result{
				IsFiltered: true,
				Reason:     dnsfilter.FilteredRebind,
				Rule:       "adguard-rebind-protection",
			}

			d.Res = s.genDNSFilterMessage(d, res)
			log.Debug("DNSRebind: Matched %s by response: %s", d.Req.Question[0].Name, host)
			return res, nil
		}
	}

	return nil, nil
}
