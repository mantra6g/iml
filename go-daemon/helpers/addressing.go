package helpers

import (
	"fmt"
	"math/big"
	"net"
	"sync"
)

// IPAllocator manages IP allocation within a CIDR block
type IPAllocator struct {
  subnet   		  net.IPNet
  broadcast 		net.IP
  lastAssigned  net.IP
  IPVersion     int // 4 for IPv4, 6 for IPv6
  mutex         sync.Mutex // Mutex to protect concurrent access
}

// NewIPAllocator creates a new allocator from a CIDR string
func NewIPAllocator(ipNet net.IPNet) (*IPAllocator, error) {
  if (ipNet.IP == nil) {
    return nil, fmt.Errorf("IPNet's IP is nil")
  }

  if ipNet.Mask == nil {
    return nil, fmt.Errorf("IPNet's Mask is nil: %v", ipNet)
  }

  // Convert IP and Mask to integers of arbitrary precision
  ipInt := ipToBigInt(ipNet.IP)
  maskInt := maskToBigInt(ipNet.Mask)

  // Make sure the IP's host part is zero
  hostPartInt := new(big.Int).AndNot(ipInt, maskInt)
  if hostPartInt.Cmp(big.NewInt(0)) != 0 {
    return nil, fmt.Errorf("invalid CIDR '%s': host part is not zero", ipNet.String())
  }

  // Calculate the broadcast address
  broadcastInt := new(big.Int).Or(ipInt, big.NewInt(0).Not(maskInt))

  // Check if the IP is IPv4 or IPv6
  ipVersion := 6
  if len(ipNet.IP) == net.IPv4len {
    ipVersion = 4
  }

  return &IPAllocator{
    subnet:    ipNet,
    broadcast: bigIntToIP(broadcastInt, ipVersion),
    lastAssigned: ipNet.IP,
    IPVersion: ipVersion,
  }, nil
}

// Next returns the next available IP, or nil if exhausted
func (a *IPAllocator) Next() (*net.IPNet, error) {
  // Lock the allocator
  a.mutex.Lock()
  defer a.mutex.Unlock()

  // Calculate the next IP based on the last one
  lastIpInt := ipToBigInt(a.lastAssigned)
  nextIpInt := new(big.Int).Add(lastIpInt, big.NewInt(1))
  
  // Check if the next IP is within the subnet range
  if nextIpInt.Cmp(ipToBigInt(a.broadcast)) == 0 {
    // No more IPs available
    return nil, fmt.Errorf("no more IPs available in subnet %s", a.subnet.String())
  }
  
  // Update the last assigned IP
  a.lastAssigned = bigIntToIP(nextIpInt, a.IPVersion)

  return &net.IPNet{
    IP:   a.lastAssigned,
    Mask: a.subnet.Mask,
  }, nil
}

// --- Helpers ---

func ipToBigInt(ip net.IP) *big.Int {
  return new(big.Int).SetBytes(ip.To16())
}

func maskToBigInt(mask net.IPMask) *big.Int {
  return new(big.Int).SetBytes(mask)
}

func bigIntToIP(i *big.Int, ipVersion int) net.IP {
  b := i.Bytes()
  if ipVersion == 4 {
    // ensure 4 bytes
    pad := make([]byte, 4-len(b))
    b = append(pad, b...)
    return net.IPv4(b[0], b[1], b[2], b[3])
  }
  // ensure 16 bytes
  pad := make([]byte, 16-len(b))
  b = append(pad, b...)
  return net.IP(b)
}