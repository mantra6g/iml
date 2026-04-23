package ipam

import (
	"fmt"
	"net/netip"
)

type AddrAllocator struct {
	BaseNetwork    netip.Prefix
	LastAssignedIP netip.Addr
}

func NewAddrAllocator(base netip.Prefix) (*AddrAllocator, error) {
	if !base.IsValid() {
		return nil, fmt.Errorf("invalid base network: %v", base)
	}
	if base != base.Masked() {
		return nil, fmt.Errorf("base network must be a valid prefix: %v", base)
	}
	return &AddrAllocator{
		BaseNetwork:    base,
		LastAssignedIP: base.Addr(),
	}, nil
}

func (a *AddrAllocator) Next() (netip.Addr, error) {
	next := a.LastAssignedIP.Next()
	isOutOfRange := !a.BaseNetwork.Contains(next)
	isBroadcast := !a.BaseNetwork.Contains(next.Next())
	if isOutOfRange || isBroadcast {
		return netip.Addr{}, fmt.Errorf("no more IPs available in network: %v", a.BaseNetwork)
	}
	a.LastAssignedIP = next
	return next, nil
}

type PrefixAllocator struct {
	BaseNetwork  netip.Prefix
	LastAssigned netip.Prefix
	PrefixLen    int
}

func NewPrefixAllocator(base netip.Prefix, subnetPrefixLen int) (*PrefixAllocator, error) {
	if !base.IsValid() {
		return nil, fmt.Errorf("invalid base network: %v", base)
	}
	if base != base.Masked() {
		return nil, fmt.Errorf("base network must be a valid prefix: %v", base)
	}
	if subnetPrefixLen <= base.Bits() || subnetPrefixLen > base.Addr().BitLen() {
		return nil, fmt.Errorf("invalid subnet prefix length: %d, must be > %d and <= %d",
			subnetPrefixLen, base.Bits(), base.Addr().BitLen())
	}
	return &PrefixAllocator{
		BaseNetwork:  base,
		LastAssigned: netip.Prefix{},
		PrefixLen:    subnetPrefixLen,
	}, nil
}

func (a *PrefixAllocator) Next() (netip.Prefix, error) {
	var nextPrefixNetAddress netip.Addr
	if a.LastAssigned.IsValid() {
		nextPrefixNetAddress = broadcastAddr(a.LastAssigned).Next()
	} else {
		nextPrefixNetAddress = a.BaseNetwork.Addr()
	}
	nextPrefix, err := nextPrefixNetAddress.Prefix(a.PrefixLen)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("failed to create prefix from address: %v", err)
	}
	if !a.BaseNetwork.Contains(nextPrefix.Addr()) {
		return netip.Prefix{}, fmt.Errorf("no more prefixes available in network: %v", a.BaseNetwork)
	}
	a.LastAssigned = nextPrefix
	return nextPrefix, nil
}

func broadcastAddr(p netip.Prefix) netip.Addr {
	addr := p.Addr()
	// Get address as 16-byte form (works for both IPv4 and IPv6)
	ip := addr.As16()
	// Number of bits in address
	bits := addr.BitLen()
	// Prefix length
	ones := p.Bits()
	// Number of host bits
	hostBits := bits - ones
	for i := 0; i < hostBits; i++ {
		// Index starting from the end of the 16-byte array
		byteIndex := 15 - (i / 8)
		bitIndex := i % 8
		ip[byteIndex] |= 1 << bitIndex
	}
	return netip.AddrFrom16(ip).Unmap()
}
