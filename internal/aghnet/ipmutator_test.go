package aghnet

import (
	"net"
	"testing"

	"github.com/AdguardTeam/golibs/netutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIPMutator_internal(t *testing.T) {
	t.Run("noop", func(t *testing.T) {
		m := NewIPMutator(true, nil)
		assert.IsType(t, &NopIPMutator{}, m)
	})

	t.Run("real", func(t *testing.T) {
		called := false
		mutFunc := func(_ net.IP) {
			called = true
		}

		m := NewIPMutator(true, mutFunc)
		require.IsType(t, &customIPMutator{}, m)

		m.Mutate(nil)

		assert.True(t, called)
	})

	t.Run("real_disabled", func(t *testing.T) {
		called := false
		mutFunc := func(_ net.IP) {
			called = true
		}

		m := NewIPMutator(false, mutFunc)
		require.IsType(t, &customIPMutator{}, m)

		m.Mutate(nil)

		assert.False(t, called)
	})
}

func TestCustomIPMutator_Mutate(t *testing.T) {
	theIP := net.IP{1, 2, 3, 4}

	testCases := []struct {
		name string
		f    IPMutFunc
		want net.IP
	}{{
		name: "zeroer",
		f: func(ip net.IP) {
			for i := range ip {
				ip[i] = 0
			}
		},
		want: net.IP{0, 0, 0, 0},
	}, {
		name: "masker",
		f: func(ip net.IP) {
			copy(ip, ip.Mask(ip.DefaultMask()))
		},
		want: net.IP{1, 0, 0, 0},
	}, {
		name: "reverser",
		f: func(ip net.IP) {
			for i, l := 0, len(ip)-1; i < l; i++ {
				ip[i], ip[l] = ip[l], ip[i]
				l--
			}
		},
		want: net.IP{4, 3, 2, 1},
	}, {
		name: "noop",
		f:    nil,
		want: netutil.CloneIP(theIP),
	}}

	for _, tc := range testCases {
		ip := netutil.CloneIP(theIP)
		m := NewIPMutator(true, tc.f)

		t.Run(tc.name, func(t *testing.T) {
			m.Mutate(ip)
			assert.True(t, ip.Equal(tc.want))
		})
	}
}
