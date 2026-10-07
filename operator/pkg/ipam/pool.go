package ipam

import (
	"errors"
	"fmt"
	"net/netip"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// ErrPoolExhausted is returned by AddrPool.Allocate when every address in the pool is taken.
var ErrPoolExhausted = errors.New("no more addresses available")

// AddrPool hands out single addresses from a prefix to named owners. Unlike AddrAllocator,
// addresses can be released and reserved again, so the pool can be rebuilt from the
// addresses already recorded in the cluster. It is safe for concurrent use.
type AddrPool struct {
	mu     sync.Mutex
	base   netip.Prefix
	owners map[netip.Addr]types.NamespacedName
	addrs  map[types.NamespacedName]netip.Addr
	// last is the most recently allocated address. The next allocation starts right after it,
	// so released addresses aren't handed out again immediately.
	last netip.Addr
}

func NewAddrPool(base netip.Prefix) (*AddrPool, error) {
	if !base.IsValid() {
		return nil, fmt.Errorf("invalid base network: %v", base)
	}
	if base != base.Masked() {
		return nil, fmt.Errorf("base network must be a valid prefix: %v", base)
	}
	return &AddrPool{
		base:   base,
		owners: map[netip.Addr]types.NamespacedName{},
		addrs:  map[types.NamespacedName]netip.Addr{},
		last:   base.Addr(),
	}, nil
}

// Prefix returns the prefix the pool allocates from.
func (p *AddrPool) Prefix() netip.Prefix {
	return p.base
}

// Contains reports whether addr can be handed out by the pool.
func (p *AddrPool) Contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !p.base.Contains(addr) || addr == p.base.Addr() {
		return false
	}
	// IPv4 broadcast addresses are never handed out.
	return !addr.Is4() || addr != broadcastAddr(p.base)
}

// Allocate returns the address held by owner, allocating a free one if it holds none.
func (p *AddrPool) Allocate(owner types.NamespacedName) (netip.Addr, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if addr, ok := p.addrs[owner]; ok {
		return addr, nil
	}
	start := p.next(p.last)
	addr := start
	for {
		if _, taken := p.owners[addr]; !taken && p.Contains(addr) {
			p.assign(addr, owner)
			p.last = addr
			return addr, nil
		}
		if addr = p.next(addr); addr == start {
			return netip.Addr{}, fmt.Errorf("%w in network %v", ErrPoolExhausted, p.base)
		}
	}
}

// Reserve assigns addr to owner. Any other address held by owner is released.
func (p *AddrPool) Reserve(addr netip.Addr, owner types.NamespacedName) error {
	addr = addr.Unmap()
	if !p.Contains(addr) {
		return fmt.Errorf("address %v is not available in network %v", addr, p.base)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if current, ok := p.owners[addr]; ok {
		if current == owner {
			return nil
		}
		return fmt.Errorf("address %v is already assigned to %v", addr, current)
	}
	p.release(owner)
	p.assign(addr, owner)
	return nil
}

// Release frees the address held by owner, if any.
func (p *AddrPool) Release(owner types.NamespacedName) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.release(owner)
}

// next returns the address after addr, wrapping around to the start of the prefix.
func (p *AddrPool) next(addr netip.Addr) netip.Addr {
	if n := addr.Next(); n.IsValid() && p.base.Contains(n) {
		return n
	}
	return p.base.Addr()
}

func (p *AddrPool) assign(addr netip.Addr, owner types.NamespacedName) {
	p.owners[addr] = owner
	p.addrs[owner] = addr
}

func (p *AddrPool) release(owner types.NamespacedName) {
	if addr, ok := p.addrs[owner]; ok {
		delete(p.owners, addr)
		delete(p.addrs, owner)
	}
}
