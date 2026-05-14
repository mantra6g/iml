package nat

import (
	"fmt"
	"net/netip"

	"github.com/coreos/go-iptables/iptables"
)

type Box interface {
	SetDestination(addr netip.Addr) error
}

type Config struct {
	PreroutingChainName  string
	PostroutingChainName string
	OwnAddress           netip.Addr
}

type box struct {
	ipt                  *iptables.IPTables
	currDest             netip.Addr
	preroutingChainName  string
	postroutingChainName string
	ownAddress           netip.Addr
}

func NewBox(c Config) (Box, error) {
	if !c.OwnAddress.IsValid() || !c.OwnAddress.Is6() {
		return nil, fmt.Errorf("invalid own address: %s", c.OwnAddress)
	}
	if c.PreroutingChainName == "" {
		return nil, fmt.Errorf("chain name is required")
	}
	ipt, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv6))
	if err != nil {
		return nil, fmt.Errorf("failed to create iptables instance: %w", err)
	}
	err = ipt.NewChain("nat", c.PreroutingChainName)
	if err != nil {
		return nil, fmt.Errorf("failed to create prerouting nat chain: %w", err)
	}
	err = ipt.NewChain("nat", c.PostroutingChainName)
	if err != nil {
		return nil, fmt.Errorf("failed to create postrouting nat chain: %w", err)
	}
	err = ipt.Insert("nat", "PREROUTING", 1, "-j", c.PreroutingChainName)
	if err != nil {
		return nil, fmt.Errorf("failed to insert rule into PREROUTING chain: %w", err)
	}
	err = ipt.Insert("nat", "POSTROUTING", 1, "-j", c.PostroutingChainName)
	if err != nil {
		return nil, fmt.Errorf("failed to insert rule into POSTROUTING chain: %w", err)
	}
	return &box{
		ipt:                  ipt,
		preroutingChainName:  c.PreroutingChainName,
		postroutingChainName: c.PostroutingChainName,
		ownAddress:           c.OwnAddress,
	}, nil
}

func (b *box) SetDestination(addr netip.Addr) error {
	if !addr.IsValid() || !addr.Is6() {
		return fmt.Errorf("invalid address: %s", addr)
	}
	err := b.ipt.ClearChain("nat", b.preroutingChainName)
	if err != nil {
		return fmt.Errorf("failed to clear prerouting nat chain: %w", err)
	}
	err = b.ipt.ClearChain("nat", b.postroutingChainName)
	if err != nil {
		return fmt.Errorf("failed to clear postrouting nat chain: %w", err)
	}
	err = b.ipt.Append("nat", b.preroutingChainName, "-d", b.ownAddress.String(), "-j", "DNAT", "--to-destination", addr.String())
	if err != nil {
		return fmt.Errorf("failed to append rule to prerouting nat chain: %w", err)
	}
	err = b.ipt.Append("nat", b.postroutingChainName, "-d", addr.String(), "-j", "MASQUERADE")
	if err != nil {
		return fmt.Errorf("failed to append rule to postrouting nat chain: %w", err)
	}
	b.currDest = addr
	return nil
}
