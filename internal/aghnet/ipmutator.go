package aghnet

import (
	"net"
	"sync/atomic"
)

// IPMutator is used to modify or left the passed IP address instance unchanged
// depending on it's internal state.
type IPMutator interface {
	// Mutate modifies the passed ip if the mutator is enabled.  It should be
	// safe for concurrent use.
	Mutate(ip net.IP)

	// SetEnabled controls if passed IP addresses should be modified.  It should
	// be safe for concurrent use.
	SetEnabled(enabled bool)
}

// NopIPMutator is an IPMutator that does nothing.
type NopIPMutator struct{}

// Mutate implements the IPMutator interface for *NopIPMutator.
func (m *NopIPMutator) Mutate(_ net.IP) {}

// SetEnabled implements the IPMutator interface for *NopIPMutator.
func (m *NopIPMutator) SetEnabled(_ bool) {}

// IPMutFunc is the signature of a function which modifies the IP address
// instance.  It should be safe for concurrent use.
type IPMutFunc func(ip net.IP)

// customIPMutator is the IPMutator that applies custom modifying method to
// the IP address.
type customIPMutator struct {
	enabled uint32
	f       IPMutFunc
}

// NewIPMutator returns the new IPMutator which modifies the IP address with f.
func NewIPMutator(enabled bool, f IPMutFunc) (m IPMutator) {
	if f == nil {
		return &NopIPMutator{}
	}

	var enabledVal uint32
	if enabled {
		enabledVal = 1
	}

	return &customIPMutator{
		enabled: enabledVal,
		f:       f,
	}
}

// Mutate implements the IPMutator interface for *customIPMutator.
func (a *customIPMutator) Mutate(ip net.IP) {
	if atomic.LoadUint32(&a.enabled) == 1 {
		a.f(ip)
	}
}

// SetEnabled implements the IPMutator interface for *customIPMutator.
func (a *customIPMutator) SetEnabled(enabled bool) {
	if enabled {
		atomic.StoreUint32(&a.enabled, 1)
	} else {
		atomic.StoreUint32(&a.enabled, 0)
	}
}
