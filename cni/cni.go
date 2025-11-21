package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"imlcni/logger"
	"net"
	"net/http"
	"os"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

func cmdAdd(cniArgs *skel.CmdArgs) (err error) {
	logger.InfoLogger().Printf("ADD called for container %s in netns %s\n", cniArgs.ContainerID, cniArgs.Netns)
	logger.DebugLogger().Printf("CNI Args: %+v\n", cniArgs)
	cniConf := IMLCNIConfig{}
	if err := json.Unmarshal(cniArgs.StdinData, &cniConf); err != nil {
		logger.ErrorLogger().Printf("Failed to parse CNI configuration: %v\n", err)
		return fmt.Errorf("failed to parse network config: %w", err)
	}

	var result types.Result
	switch cniConf.Args.CNI.AppType {
	case "network_function":
		logger.InfoLogger().Printf("Deploying network function for container %s\n", cniArgs.ContainerID)

		configRequest := NFConfigRequest{
			VnfID: cniConf.Args.CNI.NfID,
			ContainerID: cniArgs.ContainerID,
		}

		// Use the NFId to request the network configuration from IML
		netConfig, err := getNFConfigFromIML(configRequest)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to get network config from IML: %v\n", err)
			return fmt.Errorf("failed to get network config from IML: %w", err)
		}

		// Deploy the network function using the configuration
		result, err = deployNetworkFunction(netConfig, cniArgs)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to deploy network function: %v\n", err)
			return fmt.Errorf("failed to deploy network function: %w", err)
		}
	case "application_function":
		logger.InfoLogger().Printf("Deploying application function for container %s\n", cniArgs.ContainerID)

		configRequest := AppConfigRequest{
			ApplicationID: cniConf.Args.CNI.AppId,
			ContainerID:   cniArgs.ContainerID,
		}

		// Use the AppId to request the network configuration from IML
		netConfig, err := getAppConfigFromIML(configRequest)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to get network config from IML: %v\n", err)
			return fmt.Errorf("failed to get network config from IML: %w", err)
		}
		// Deploy the application function using the configuration
		result, err = deployApplicationFunction(netConfig, cniArgs)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to deploy application function: %v\n", err)
			return fmt.Errorf("failed to deploy application function: %w", err)
		}
	default:
		logger.ErrorLogger().Printf("Unknown app type: %s\n", cniConf.Args.CNI.AppType)
		return fmt.Errorf("unknown app type: %s", cniConf.Args.CNI.AppType)
	}

	return types.PrintResult(result, cniConf.CNIVersion)
}

func deployApplicationFunction(netConfig *AppConfigResponse, cniArgs *skel.CmdArgs) (types.Result, error) {
	logger.DebugLogger().Printf("Deploying application function with config: %+v\n", netConfig)

	// Parse the IP address from the response
	ipNet, err := netlink.ParseIPNet(netConfig.IPNet)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP address: %s", netConfig.IPNet)
	}

	// Parse the destination network from the response
	clusterNet, err := netlink.ParseIPNet(netConfig.ClusterCIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster network: %s", netConfig.ClusterCIDR)
	}

	// Parse the gateway IP from the response
	gwIP := net.ParseIP(netConfig.GatewayIP)
	if gwIP == nil {
		return nil, fmt.Errorf("failed to parse gateway IP: %s", netConfig.GatewayIP)
	}

	// Get the host's network namespace
	hostNs, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get host netns: %w", err)
	}

	// Create a veth pair for the container
	// The container interface is called "iml0"
	// The host interface is called "nfr-..."
	imlInterface := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "iml0",
			MTU:  1500,
		},
		PeerName:         netConfig.IfaceName,
		PeerMTU:          1500,
	}

	// Change to the container namespace
	err = execInsideNs(cniArgs.Netns, func() error {
		// Add the veth pair
		if err := netlink.LinkAdd(imlInterface); err != nil {
			return fmt.Errorf("failed to create veth pair: %w", err)
		}
		peerInterface, err := netlink.LinkByName(imlInterface.PeerName)
		if err != nil {
			return fmt.Errorf("failed to get host interface %s: %w", imlInterface.PeerName, err)
		}
		// Move the peer interface to the host namespace
		if err := netlink.LinkSetNsFd(peerInterface, int(hostNs)); err != nil {
			return fmt.Errorf("failed to move host interface %s to host netns: %w", imlInterface.PeerName, err)
		}
		// Set both ends of the veth pair up
		if err := netlink.LinkSetUp(imlInterface); err != nil {
			return fmt.Errorf("failed to set container interface up: %w", err)
		}
		// Set the container interface's IP address
		if err := netlink.AddrAdd(imlInterface, &netlink.Addr{IPNet: ipNet}); err != nil {
			return fmt.Errorf("failed to add IP address to container interface %s: %w", imlInterface.Name, err)
		}

		// Create route to the destination network
		routeLink := &netlink.Route{
			Dst: clusterNet,
			Gw:  gwIP,
			Scope: netlink.SCOPE_UNIVERSE,
			Family: nl.FAMILY_V6,
		}

		// Add the route inside the container's network namespace
		if err := netlink.RouteAdd(routeLink); err != nil {
			return fmt.Errorf("failed to add route: %w", err)
		}
		return nil
	})
	hostNs.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to execute inside container netns %s: %w", cniArgs.Netns, err)
	}

	hostLink, err := netlink.LinkByName(imlInterface.PeerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get host interface %s: %w", imlInterface.PeerName, err)
	}

	bridge, err := netlink.LinkByName(netConfig.BridgeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bridge interface: %w", err)
	}

	err = netlink.LinkSetMaster(hostLink, bridge)
	if err != nil {
		return nil, fmt.Errorf("failed to set host interface %s master to bridge: %w", imlInterface.PeerName, err)
	}

	err = netlink.LinkSetUp(hostLink)
	if err != nil {
		return nil, fmt.Errorf("failed to set host interface %s up: %w", imlInterface.PeerName, err)
	}

	intfIndex := 0
	result := &current.Result{
		Interfaces: []*current.Interface{
			{
				Name:    imlInterface.Name,
				Sandbox: cniArgs.Netns,
			},
		},
		IPs: []*current.IPConfig{
			{
				Interface: &intfIndex,
				Address:   *ipNet,
				Gateway:   gwIP,
			},
		},
		Routes: []*types.Route{
			{
				Dst: *clusterNet,
				GW:  gwIP,
			},
		},
	}

	return result, nil
}

func deployNetworkFunction(netConfig *NFConfigResponse, cniArgs *skel.CmdArgs) (types.Result, error) {
	logger.DebugLogger().Printf("Deploying network function with config: %+v\n", netConfig)

	// Parse the IP address from the response
	ipNet, err := netlink.ParseIPNet(netConfig.IPNet)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP address: %s", netConfig.IPNet)
	}

	// Parse the SID address from the response
	var sidList = []*net.IPNet{}
	for _, sidStr := range netConfig.SIDs {
		sid, err := netlink.ParseIPNet(sidStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SID address: %s", sidStr)
		}
		sidList = append(sidList, sid)
	}

	// Parse the destination network from the response
	clusterNet, err := netlink.ParseIPNet(netConfig.ClusterCIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster network: %s", netConfig.ClusterCIDR)
	}

	// Parse the gateway IP from the response
	gwIP := net.ParseIP(netConfig.GatewayIP)
	if gwIP == nil {
		return nil, fmt.Errorf("failed to parse gateway IP: %s", netConfig.GatewayIP)
	}

	// Get the host's network namespace
	hostNs, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get host netns: %w", err)
	}

	// Create a veth pair for the container
	// The container interface is called "iml0"
	// The host interface is called "nfr-..."
	imlInterface := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "iml0",
			MTU:  1500,
		},
		PeerName: netConfig.IfaceName,
		PeerMTU:  1500,
	}

	// Change to the container namespace
	err = execInsideNs(cniArgs.Netns, func() error {
		// Set up ip forwarding and SRv6
		if err := os.WriteFile("/proc/sys/net/ipv6/conf/all/forwarding", []byte("1"), 0644); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
		}
		if err := os.WriteFile("/proc/sys/net/ipv6/conf/all/seg6_enabled", []byte("1"), 0644); err != nil {
			return fmt.Errorf("failed to enable SRv6: %w", err)
		}

		// Add the veth pair
		if err := netlink.LinkAdd(imlInterface); err != nil {
			return fmt.Errorf("failed to create veth pair: %w", err)
		}
		peerInterface, err := netlink.LinkByName(imlInterface.PeerName)
		if err != nil {
			return fmt.Errorf("failed to get host interface %s: %w", imlInterface.PeerName, err)
		}
		// Move the peer interface to the host namespace
		if err := netlink.LinkSetNsFd(peerInterface, int(hostNs)); err != nil {
			return fmt.Errorf("failed to move host interface %s to host netns: %w", imlInterface.PeerName, err)
		}
		// Set both ends of the veth pair up
		if err := netlink.LinkSetUp(imlInterface); err != nil {
			return fmt.Errorf("failed to set container interface up: %w", err)
		}
		// Set the container interface's subnet IP address
		// ip -6 addr add <IP>/<PREFIXLEN> dev iml0
		if err := netlink.AddrAdd(imlInterface, &netlink.Addr{IPNet: ipNet}); err != nil {
			return fmt.Errorf("failed to add IP address to container interface %s: %w", imlInterface.Name, err)
		}
		// Set the container interface's SID address
		// ip -6 addr add <SID>/<PREFIXLEN> dev iml0 anycast
		for _, sid := range sidList {
			if err := netlink.AddrAdd(imlInterface, &netlink.Addr{
				IPNet: sid,
				Flags: unix.IFA_ANYCAST,
			}); err != nil {
				return fmt.Errorf("failed to add SID address to container interface %s: %w", imlInterface.Name, err)
			}
		}
		// Create route to the destination network
		// ip -6 route add <DESTINATION> via <GATEWAY> src <IP>
		routeLink := &netlink.Route{
			Dst: clusterNet,
			Gw:  gwIP,
			Scope: netlink.SCOPE_UNIVERSE,
			Family: nl.FAMILY_V6,
		}
		// Add the route inside the container's network namespace
		if err := netlink.RouteAdd(routeLink); err != nil {
			return fmt.Errorf("failed to add route: %w", err)
		}
		// Removed because this needs to be implemented by the NF itself
		// // Add SRv6 End.X route
		// // ip -6 route add <SID> dev lo encap seg6local action End.X nh6 <Gateway> dev <Container interface>
		// end_x_flags := [nl.SEG6_LOCAL_MAX]bool{}
		// end_x_flags[nl.SEG6_LOCAL_ACTION] = true
		// end_x_flags[nl.SEG6_LOCAL_NH6] = true
		// srv6EndRoute := &netlink.Route{
		// 	Dst:      sid,
		// 	LinkIndex: imlInterface.Attrs().Index,
		// 	Scope: netlink.SCOPE_UNIVERSE,
		// 	Encap: &netlink.SEG6LocalEncap{
		// 		Flags:     end_x_flags,
		// 		Action: nl.SEG6_LOCAL_ACTION_END_X,
		// 		In6Addr: gwIP,
		// 	},
		// }
		// if err := netlink.RouteAdd(srv6EndRoute); err != nil {
		// 	return fmt.Errorf("failed to add SRv6 End route: %w", err)
		// }
		return nil
	})
	hostNs.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to execute inside container netns %s: %w", cniArgs.Netns, err)
	}

	hostLink, err := netlink.LinkByName(imlInterface.PeerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get host interface %s: %w", imlInterface.PeerName, err)
	}

	bridge, err := netlink.LinkByName(netConfig.BridgeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bridge interface: %w", err)
	}

	err = netlink.LinkSetMaster(hostLink, bridge)
	if err != nil {
		return nil, fmt.Errorf("failed to set host interface %s master to bridge: %w", imlInterface.PeerName, err)
	}

	err = netlink.LinkSetUp(hostLink)
	if err != nil {
		return nil, fmt.Errorf("failed to set host interface %s up: %w", imlInterface.PeerName, err)
	}

	intfIndex := 0
	result := &current.Result{
		Interfaces: []*current.Interface{
			{
				Name:    imlInterface.Name,
				Sandbox: cniArgs.Netns,
			},
		},
		IPs: []*current.IPConfig{
			{
				Interface: &intfIndex,
				Address:   *ipNet,
				Gateway:   gwIP,
			},
		},
		Routes: []*types.Route{
			{
				Dst: *clusterNet,
				GW:  gwIP,
			},
		},
	}

	return result, nil
}

// Execute function inside a namespace
func execInsideNs(netnsPath string, function func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	stdNetns, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get current netns: %w", err)
	}
	defer stdNetns.Close()

	containerNs, err := netns.GetFromPath(netnsPath)
	if err != nil {
		return fmt.Errorf("failed to open netns %s: %w", netnsPath, err)
	}
	defer netns.Set(stdNetns)

	err = netns.Set(containerNs)
	if err != nil {
		return fmt.Errorf("failed to set netns %s: %w", netnsPath, err)
	}
	return function()
}

func getAppConfigFromIML(payload AppConfigRequest) (*AppConfigResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	resp, err := http.Post(
		"http://localhost:7623/api/v1/cni/app/register",
		"application/json", bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("received non-2XX response: %s", resp.Status)
	}
	var configResponse AppConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &configResponse, nil
}

func getNFConfigFromIML(configRequest NFConfigRequest) (*NFConfigResponse, error) {
	data, err := json.Marshal(configRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	resp, err := http.Post(
		"http://localhost:7623/api/v1/cni/vnf/register",
		"application/json", bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("received non-2XX response: %s", resp.Status)
	}
	var configResponse NFConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &configResponse, nil
}

func tearDownNetworkFunction(cniArgs *skel.CmdArgs) error {
	logger.InfoLogger().Printf("Tearing down network function for container %s in netns %s\n", cniArgs.ContainerID, cniArgs.Netns)

	// Notify IML of the network function teardown
	configRequest := NfTeardownRequest{
		ContainerID: cniArgs.ContainerID,
	}
	err := notifyIMLOfNfTeardown(configRequest)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to notify IML of teardown: %v\n", err)
		return fmt.Errorf("failed to notify IML of teardown: %w", err)
	}

	// Change to the container namespace
	err = execInsideNs(cniArgs.Netns, func() error {
		// Get the peer interface by name inside the container's namespace
		intf, err := netlink.LinkByName("iml0")
		if err != nil {
			return fmt.Errorf("failed to get interface %s in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		// Set the interface down
		if err := netlink.LinkSetDown(intf); err != nil {
			return fmt.Errorf("failed to set interface %s down in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		// Delete the interface
		if err := netlink.LinkDel(intf); err != nil {
			return fmt.Errorf("failed to delete interface %s in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		return nil
	})
	if err != nil {
		logger.ErrorLogger().Printf("Failed to execute inside netns %s: %v\n", cniArgs.Netns, err)
		return fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
	}

	return nil
}

func tearDownApplicationFunction(cniArgs *skel.CmdArgs) error {
	logger.InfoLogger().Printf("Tearing down application function for container %s in netns %s\n", cniArgs.ContainerID, cniArgs.Netns)

	// Notify IML of the application function teardown
	configRequest := AppTeardownRequest{
		ContainerID: cniArgs.ContainerID,
	}
	err := notifyIMLOfAppTeardown(configRequest)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to notify IML of teardown: %v\n", err)
		return fmt.Errorf("failed to notify IML of teardown: %w", err)
	}

	// Change to the container namespace
	err = execInsideNs(cniArgs.Netns, func() error {
		// Get the peer interface by name inside the container's namespace
		intf, err := netlink.LinkByName("iml0")
		if err != nil {
			// If the interface does not exist, consider it already torn down
			return nil
		}

		// Set the interface down
		if err := netlink.LinkSetDown(intf); err != nil {
			return fmt.Errorf("failed to set interface %s down in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		// Delete the interface
		if err := netlink.LinkDel(intf); err != nil {
			return fmt.Errorf("failed to delete interface %s in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		return nil
	})
	if err != nil {
		logger.ErrorLogger().Printf("Failed to execute inside netns %s: %v\n", cniArgs.Netns, err)
		return fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
	}

	return nil
}

func notifyIMLOfAppTeardown(request AppTeardownRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %w", err)
	}

	resp, err := http.Post(
		"http://localhost:7623/api/v1/cni/app/teardown",
		"application/json", bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-2XX response: %s", resp.Status)
	}
	logger.InfoLogger().Printf("Successfully notified IML of teardown for app with container id %s\n", request.ContainerID)
	return nil
}

func notifyIMLOfNfTeardown(request NfTeardownRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %w", err)
	}

	resp, err := http.Post(
		"http://localhost:7623/api/v1/cni/vnf/teardown",
		"application/json", bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-2XX response: %s", resp.Status)
	}
	logger.InfoLogger().Printf("Successfully notified IML of teardown for vnf with container id %s\n", request.ContainerID)
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	logger.InfoLogger().Printf("DEL called for container %s\n", args.ContainerID)
	
	cniConf := IMLCNIConfig{}
	if err := json.Unmarshal(args.StdinData, &cniConf); err != nil {
		logger.ErrorLogger().Printf("Failed to parse CNI configuration: %v\n", err)
		return fmt.Errorf("failed to parse network config: %w", err)
	}

	switch cniConf.Args.CNI.AppType {
		case "network_function":
			logger.InfoLogger().Printf("Tearing down network function for container %s\n", args.ContainerID)
			if err := tearDownNetworkFunction(args); err != nil {
				logger.ErrorLogger().Printf("Failed to tear down network function: %v\n", err)
				return fmt.Errorf("failed to tear down network function: %w", err)
			}
		case "application_function":
			logger.InfoLogger().Printf("Tearing down application function for container %s\n", args.ContainerID)
			if err := tearDownApplicationFunction(args); err != nil {
				logger.ErrorLogger().Printf("Failed to tear down application function: %v\n", err)
				return fmt.Errorf("failed to tear down application function: %w", err)
			}
		default:
			logger.ErrorLogger().Printf("Unknown app type: %s\n", cniConf.Args.CNI.AppType)
			return fmt.Errorf("unknown app type: %s", cniConf.Args.CNI.AppType)
	}
	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	logger.InfoLogger().Printf("CHECK called for container %s\n", args.ContainerID)
	fmt.Fprintf(os.Stderr, "CHECK not implemented\n")
	return nil
}

func versionInfo() version.PluginInfo {
	return version.PluginSupports("0.3.1", "0.4.0")
}

func main() {
	logger.InfoLogger().Println("IML CNI Plugin starting...")
	skel.PluginMainFuncs(skel.CNIFuncs{
		Add:   cmdAdd,
		Del:   cmdDel,
		Check: cmdCheck,
	}, versionInfo(), "CNI Plugin for IML")
}
