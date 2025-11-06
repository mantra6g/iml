package router

// import (
// 	"fmt"
// 	"net"
// )


// type Subnet struct {
// 	Network net.IPNet
// 	Gateway net.IP
// 	Bridge  string
// }

// func (r *RouterService) RequestAppSubnet() (*Subnet, error) {
// 	r.mu.Lock()
// 	defer r.mu.Unlock()
// 	if r.nextAppRange == nil {
// 		return nil, fmt.Errorf("no more subnets available in the application subnet range")
// 	}

// 	// Get a new /64 subnet from the AppSubnet range
// 	network := r.nextAppRange.IPNet
// 	gatewayIP, err := r.nextAppRange.NextIP(network.IP)
// 	if err != nil {
// 		return nil, fmt.Errorf("cannot obtain gateway ip as addresses were exhausted")
// 	}
// 	subnet := &Subnet{
// 		Network: network,
// 		Gateway: gatewayIP,
// 	}

// 	bridgeName, err := r.dataplane.AddSubnet(gatewayIP, network)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to add application subnet to dataplane: %w", err)
// 	}
// 	subnet.Bridge = bridgeName

// 	// Advance to the next /64 subnet
// 	nextAppRange := r.nextAppRange.NextNet(64)
// 	ok := r.appSubnet.Contains(nextAppRange.IP())
// 	if !ok {
// 		r.nextAppRange = nil
// 		return subnet, nil
// 	}

// 	r.nextAppRange = &nextAppRange
// 	return subnet, nil
// }

// func (r *RouterService) RequestNFSubnet() (*Subnet, error) {
// 	r.mu.Lock()
// 	defer r.mu.Unlock()
// 	if r.nextVNFRange == nil {
// 		return nil, fmt.Errorf("no more subnets available in the VNF subnet range")
// 	}

// 	// Get a new /64 subnet from the NFSubnet range
// 	network := r.nextVNFRange.IPNet
// 	gatewayIP, err := r.nextVNFRange.NextIP(network.IP)
// 	if err != nil {
// 		return nil, fmt.Errorf("cannot obtain gateway ip as addresses were exhausted")
// 	}
// 	subnet := &Subnet{
// 		Network: network,
// 		Gateway: gatewayIP,
// 	}

// 	bridgeName, err := r.dataplane.AddSubnet(gatewayIP, network)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to add VNF subnet to dataplane: %w", err)
// 	}
// 	subnet.Bridge = bridgeName

// 	// Advance to the next /64 subnet
// 	nextVNFRange := r.nextVNFRange.NextNet(64)
// 	ok := r.vnfSubnet.Contains(nextVNFRange.IP())
// 	if !ok {
// 		r.nextVNFRange = nil
// 		return subnet, nil
// 	}

// 	r.nextVNFRange = &nextVNFRange
// 	return subnet, nil
// }