package dataplane

import (
	"fmt"
	"net"
	"sync"

	"github.com/c-robinson/iplib/v2"
)

type Subnet4Allocator struct {
	baseNet *net.IPNet
	last    *iplib.Net4
	prefix  int
	mu      sync.Mutex
}

// Creates a new SubnetAllocator.
//
// It uses the given base ipv6 network to create subnets of the specified prefix length.
func NewSubnet4Allocator(baseNet *net.IPNet, prefix int) (*Subnet4Allocator, error) {
	if baseNet == nil {
		return nil, fmt.Errorf("base network is nil")
	}
	if len(baseNet.IP) != net.IPv4len {
		return nil, fmt.Errorf("base network is not a valid IPv4 network")
	}
	if prefix <= 0 || prefix > 32 {
		return nil, fmt.Errorf("invalid prefix length: %d", prefix)
	}
	return &Subnet4Allocator{
		baseNet: baseNet,
		prefix:  prefix,
	}, nil
}
func (sa *Subnet4Allocator) Allocate() (*net.IPNet, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	var subnet iplib.Net4
	if sa.last == nil {
		subnet = iplib.NewNet4(sa.baseNet.IP, sa.prefix)
	} else {
		subnet = sa.last.NextNet(sa.prefix)
	}
	if !sa.baseNet.Contains(subnet.IPNet.IP) {
		return nil, fmt.Errorf("no more subnets available in the base network")
	}
	sa.last = &subnet
	return &subnet.IPNet, nil
}

type Subnet6Allocator struct {
	baseNet *net.IPNet
	last    *iplib.Net6
	prefix  int
	mu      sync.Mutex
}

// Creates a new SubnetAllocator.
//
// It uses the given base ipv6 network to create subnets of the specified prefix length.
func NewSubnet6Allocator(baseNet *net.IPNet, prefix int) (*Subnet6Allocator, error) {
	if baseNet == nil {
		return nil, fmt.Errorf("base network is nil")
	}
	if baseNet.IP.To16() == nil || baseNet.IP.To4() != nil {
		return nil, fmt.Errorf("base network is not a valid IPv6 network")
	}
	if prefix <= 0 || prefix > 128 {
		return nil, fmt.Errorf("invalid prefix length: %d", prefix)
	}
	return &Subnet6Allocator{
		baseNet: baseNet,
		prefix:  prefix,
	}, nil
}
func (sa *Subnet6Allocator) Allocate() (*net.IPNet, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	var subnet iplib.Net6
	if sa.last == nil {
		subnet = iplib.NewNet6(sa.baseNet.IP, sa.prefix, 0)
	} else {
		subnet = sa.last.NextNet(sa.prefix)
	}
	if !sa.baseNet.Contains(subnet.IPNet.IP) {
		return nil, fmt.Errorf("no more subnets available in the base network")
	}
	sa.last = &subnet
	return &subnet.IPNet, nil
}

type IPv6Allocator struct {
	baseNet iplib.Net6
	lastIP  net.IP
	mu      sync.Mutex
}

type IPv4Allocator struct {
	baseNet iplib.Net4
	lastIP  net.IP
	mu      sync.Mutex
}

func NewIPv4Allocator(baseNet *net.IPNet) (*IPv4Allocator, error) {
	if baseNet == nil {
		return nil, fmt.Errorf("base network is nil")
	}
	if len(baseNet.IP) != net.IPv4len {
		return nil, fmt.Errorf("base network is not a valid IPv4 network")
	}
	maskLen, _ := baseNet.Mask.Size()
	net4 := iplib.NewNet4(baseNet.IP, maskLen)
	return &IPv4Allocator{
		baseNet: net4,
	}, nil
}

func NewIPv6Allocator(baseNet *net.IPNet) (*IPv6Allocator, error) {
	if baseNet == nil {
		return nil, fmt.Errorf("base network is nil")
	}
	if baseNet.IP.To16() == nil || baseNet.IP.To4() != nil {
		return nil, fmt.Errorf("base network is not a valid IPv6 network")
	}
	maskLen, _ := baseNet.Mask.Size()
	net6 := iplib.NewNet6(baseNet.IP, maskLen, 0)
	return &IPv6Allocator{
		baseNet: net6,
	}, nil
}

func (ipAlloc *IPv6Allocator) Allocate() (ipNet *net.IPNet, err error) {
	ipAlloc.mu.Lock()
	defer ipAlloc.mu.Unlock()

	var newIP net.IP
	if ipAlloc.lastIP == nil {
		newIP, err = ipAlloc.baseNet.NextIP(ipAlloc.baseNet.IP())
	} else {
		newIP, err = ipAlloc.baseNet.NextIP(ipAlloc.lastIP)
		if err != nil {
			return nil, fmt.Errorf("no more IPs available in the base network: %w", err)
		}
	}
	ipAlloc.lastIP = newIP
	return &net.IPNet{
		IP:   newIP,
		Mask: ipAlloc.baseNet.Mask(),
	}, nil
}

func (ipAlloc *IPv4Allocator) Allocate() (ipNet *net.IPNet, err error) {
	ipAlloc.mu.Lock()
	defer ipAlloc.mu.Unlock()

	var newIP net.IP
	if ipAlloc.lastIP == nil {
		newIP, err = ipAlloc.baseNet.NextIP(ipAlloc.baseNet.IP())
	} else {
		newIP, err = ipAlloc.baseNet.NextIP(ipAlloc.lastIP)
		if err != nil {
			return nil, fmt.Errorf("no more IPs available in the base network: %w", err)
		}
	}
	ipAlloc.lastIP = newIP
	return &net.IPNet{
		IP:   newIP,
		Mask: ipAlloc.baseNet.Mask(),
	}, nil
}

type TableAllocator struct {
	firstTableID uint32
	lastAssigned uint32
	mu           sync.Mutex
}

func NewTableAllocator(firstID uint32) (*TableAllocator, error) {
	if firstID <= 0 {
		return nil, fmt.Errorf("table ID must be positive: %d", firstID)
	}
	if firstID <= 255 {
		return nil, fmt.Errorf("table ID must be greater than 255: %d", firstID)
	}
	return &TableAllocator{
		firstTableID: firstID,
		lastAssigned: firstID - 1,
	}, nil
}
func (ta *TableAllocator) Allocate() (uint32, error) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	ta.lastAssigned++
	return ta.lastAssigned, nil
}
