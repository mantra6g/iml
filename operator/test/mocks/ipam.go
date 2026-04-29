package mocks

import (
	"net/netip"

	"github.com/mantra6g/iml/operator/pkg/ipam"
)

type fakePrefixAllocator struct {
	returns netip.Prefix
}

func NewFakePrefixAllocator(returns netip.Prefix) ipam.PrefixAllocator {
	return &fakePrefixAllocator{returns: returns}
}

func (f *fakePrefixAllocator) Next() (netip.Prefix, error) {
	return f.returns, nil
}
