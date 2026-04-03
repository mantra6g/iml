package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"iml-daemon/pkg/netutils"
	"net"
	"net/netip"
)

func CreateLinkLocalAddrFromMAC(mac net.HardwareAddr) (*net.IPNet, error) {
	var eui64 []byte
	switch len(mac) {
	case 6:
		// Convert 48-bit MAC to EUI-64 by inserting ff:fe in the middle
		eui64 = make([]byte, 8)
		copy(eui64[0:3], mac[0:3])
		eui64[3] = 0xff
		eui64[4] = 0xfe
		copy(eui64[5:], mac[3:6])
	case 8:
		// Already EUI-64
		eui64 = make([]byte, 8)
		copy(eui64, mac)
	default:
		return nil, fmt.Errorf("expected MAC size is 6 bytes (48-bit) or 8 bytes (EUI-64), instead got %d bytes", len(mac))
	}

	// Flip the universal/local bit (the 7th bit of the first byte)
	eui64[0] ^= 0x02

	// Build IPv6 address: fe80::/64 + interface ID (EUI-64)
	ip := make(net.IP, net.IPv6len) // 16 bytes
	ip[0] = 0xfe
	ip[1] = 0x80
	// bytes 2..7 are zero
	ip[2] = 0x00
	ip[3] = 0x00
	ip[4] = 0x00
	ip[5] = 0x00
	ip[6] = 0x00
	ip[7] = 0x00
	copy(ip[8:], eui64) // interface ID placed in lower 64 bits

	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(64, 128),
	}, nil
}

// GenerateRandomName creates "<prefix>-<hex string>" with exactly length hex chars
func GenerateRandomName(prefix string, length uint) (string, error) {
	// Each byte = 2 hex chars, so we need length/2 bytes (rounded up)
	byteLen := (length + 1) / 2
	bytes := make([]byte, byteLen)

	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	hexStr := hex.EncodeToString(bytes)

	// Trim in case length is odd
	return fmt.Sprintf("%s-%s", prefix, hexStr[:length]), nil
}

// GetVRFName generates a name for the vrf based on the table number, for example "vrf-1001" for a vrf with table 1001
func GetVRFName(tableID uint32) string {
	return fmt.Sprintf("vrf-%d", tableID)
}

// GetVRFGatewayName generates a name for the gateway of a VRF
func GetVRFGatewayName(tableID uint32) string {
	return fmt.Sprintf("gw-%d", tableID)
}

func ParseDualStackGatewayFromStrings(gwStrings []string) (netutils.DualStackGateway, error) {
	addrs, err := ParseDualStackAddressFromStrings(gwStrings)
	if err != nil {
		return netutils.DualStackGateway{}, err
	}
	return netutils.DualStackGateway{
		IPv4Gateway: addrs.IPv4,
		IPv6Gateway: addrs.IPv6,
	}, nil
}

func ParseDualStackAddressFromStrings(ipStrings []string) (netutils.DualStackAddress, error) {
	result := netutils.DualStackAddress{}
	targetIPs := make([]netip.Addr, 0, len(ipStrings))
	if len(ipStrings) > 2 {
		return result, fmt.Errorf("too many IP addresses provided: expected at most 2 but got %d", len(ipStrings))
	}
	for _, targetIP := range ipStrings {
		ip, err := netip.ParseAddr(ipStrings[0])
		if err != nil {
			return result, fmt.Errorf("invalid IP address: %s", targetIP)
		}
		targetIPs = append(targetIPs, ip)
	}
	for i := range targetIPs {
		isIPv4 := targetIPs[i].Is4()
		if isIPv4 {
			if result.IPv4 != nil {
				return result, fmt.Errorf("multiple IPv4 addresses provided: %s and %s", result.IPv4, targetIPs[i])
			}
			result.IPv4 = targetIPs[i].AsSlice()
			continue
		}
		if result.IPv6 != nil {
			return result, fmt.Errorf("multiple IPv6 addresses provided: %s and %s", result.IPv6, targetIPs[i])
		}
		result.IPv6 = targetIPs[i].AsSlice()
	}
	return result, nil
}

func ParseDualStackNetworkFromStrings(networkStrings []string) (netutils.DualStackNetwork, error) {
	result := netutils.DualStackNetwork{}
	targetNets := make([]netip.Prefix, 0, len(networkStrings))
	if len(networkStrings) > 2 {
		return result, fmt.Errorf(
			"too many network addresses provided: expected at most 2 but got %d", len(networkStrings))
	}
	for _, networkString := range networkStrings {
		prefix, err := netip.ParsePrefix(networkStrings[0])
		if err != nil {
			return result, fmt.Errorf("invalid IP address: %s", networkString)
		}
		targetNets = append(targetNets, prefix)
	}
	for i := range targetNets {
		isIPv4 := targetNets[i].Addr().Is4()
		if isIPv4 {
			if result.IPv4Net != nil {
				return result, fmt.Errorf("multiple IPv4 networks provided: %s and %s", result.IPv4Net, targetNets[i])
			}
			result.IPv4Net = &net.IPNet{
				IP:   net.IP(targetNets[i].Addr().AsSlice()),
				Mask: net.CIDRMask(targetNets[i].Bits(), targetNets[i].Addr().BitLen()),
			}
			continue
		}
		if result.IPv6Net != nil {
			return result, fmt.Errorf("multiple IPv6 networks provided: %s and %s", result.IPv6Net, targetNets[i])
		}
		result.IPv6Net = &net.IPNet{
			IP:   net.IP(targetNets[i].Addr().AsSlice()),
			Mask: net.CIDRMask(targetNets[i].Bits(), targetNets[i].Addr().BitLen()),
		}
	}
	return result, nil
}

func ParsePrefixes(prefixStrings []string) ([]netip.Prefix, error) {
	cidrs := make([]netip.Prefix, 0, len(prefixStrings))
	for _, cidr := range prefixStrings {
		parsedPrefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR: %s", cidr)
		}
		cidrs = append(cidrs, parsedPrefix)
	}
	return cidrs, nil
}

func ParseAddresses(addressStrings []string) ([]netip.Addr, error) {
	addresses := make([]netip.Addr, 0, len(addressStrings))
	for _, ip := range addressStrings {
		parsedIP, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, parsedIP)
	}
	return addresses, nil
}

func ElementsMatchInAnyOrder[T comparable](p1, p2 []T) bool {
	if len(p1) != len(p2) {
		return false
	}
	counts := make(map[T]int)
	for _, v := range p1 {
		counts[v]++
	}
	for _, v := range p2 {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}
