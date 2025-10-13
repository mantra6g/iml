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
	"github.com/vishvananda/netns"
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
	ipNet, err := netlink.ParseIPNet(netConfig.IP)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP address: %s", netConfig.IP)
	}

	// TODO: Check if a MAC address is needed from IML
	// Parse the MAC address from the response
	// macAddr, err := net.ParseMAC(netConfig.MacAddress)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to parse MAC address: %s", netConfig.MacAddress)
	// }

	// Parse the destination network from the response
	dstNet, err := netlink.ParseIPNet(netConfig.Route.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to parse destination route: %s", netConfig.Route.Destination)
	}

	// Parse the gateway IP from the response
	gwIP := net.ParseIP(netConfig.Route.GatewayIP)
	if gwIP == nil {
		return nil, fmt.Errorf("failed to parse gateway IP: %s", netConfig.Route.GatewayIP)
	}

	// Get the host's network namespace
	hostNs, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get host netns: %w", err)
	}
	defer hostNs.Close()

	// Create a veth pair for the container
	// The container interface is called "iml0"
	// The host interface is called "nfr-..."
	imlInterface := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "iml0",
			MTU:  1500,
		},
		PeerName:         netConfig.PeerName,
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
			return fmt.Errorf("failed to get peer interface %s: %w", imlInterface.PeerName, err)
		}
		// Move the peer interface to the host namespace
		if err := netlink.LinkSetNsFd(peerInterface, int(hostNs)); err != nil {
			return fmt.Errorf("failed to move peer interface %s to host netns: %w", imlInterface.PeerName, err)
		}
		// Set both ends of the veth pair up
		if err := netlink.LinkSetUp(imlInterface); err != nil {
			return fmt.Errorf("failed to set veth interface up: %w", err)
		}
		// Get the peer interface of the veth pair
		containerLink, err := netlink.LinkByName(imlInterface.Name)
		if err != nil {
			return fmt.Errorf("failed to get peer interface %s: %w", imlInterface.Name, err)
		}
		// Set the peer interface's IP address
		if err := netlink.AddrAdd(containerLink, &netlink.Addr{IPNet: ipNet}); err != nil {
			return fmt.Errorf("failed to add IP address to peer interface %s: %w", imlInterface.Name, err)
		}

		// Create route to the destination network
		routeLink := &netlink.Route{
			Dst: dstNet,
			Gw:  gwIP,
			Src: ipNet.IP,
			Scope: netlink.SCOPE_UNIVERSE,
		}

		// Add the route inside the container's network namespace
		if err := netlink.RouteChange(routeLink); err != nil {
			return fmt.Errorf("failed to add route: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
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
			},
		},
		Routes: []*types.Route{
			{
				Dst: *dstNet,
				GW:  gwIP,
			},
		},
	}

	return result, nil
}

func deployNetworkFunction(netConfig *NFConfigResponse, cniArgs *skel.CmdArgs) (types.Result, error) {
	logger.DebugLogger().Printf("Deploying network function with config: %+v\n", netConfig)

	// Parse the SID from the response
	sid, err := netlink.ParseIPNet(netConfig.SID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SID: %s", netConfig.SID)
	}

	// TODO: Check if a MAC address is needed from IML
	// Parse the MAC address from the response
	// macAddr, err := net.ParseMAC(netConfig.MacAddress)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to parse MAC address: %s", netConfig.MacAddress)
	// }

	// Parse the destination network from the response
	dstNet, err := netlink.ParseIPNet(netConfig.Route.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to parse destination route: %s", netConfig.Route.Destination)
	}

	// Parse the gateway IP from the response
	gwIP := net.ParseIP(netConfig.Route.GatewayIP)
	if gwIP == nil {
		return nil, fmt.Errorf("failed to parse gateway IP: %s", netConfig.Route.GatewayIP)
	}

	// Get the host's network namespace
	hostNs, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get host netns: %w", err)
	}
	defer hostNs.Close()

	// Create a veth pair for the container
	// The container interface is called "iml0"
	// The host interface is called "nfr-..."
	imlInterface := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "iml0",
			MTU:  1500,
		},
		PeerName:         netConfig.PeerName,
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
			return fmt.Errorf("failed to get peer interface %s: %w", imlInterface.PeerName, err)
		}
		// Move the peer interface to the host namespace
		if err := netlink.LinkSetNsFd(peerInterface, int(hostNs)); err != nil {
			return fmt.Errorf("failed to move peer interface %s to host netns: %w", imlInterface.PeerName, err)
		}
		// Set both ends of the veth pair up
		if err := netlink.LinkSetUp(imlInterface); err != nil {
			return fmt.Errorf("failed to set veth interface up: %w", err)
		}
		// Get the peer interface of the veth pair
		containerLink, err := netlink.LinkByName(imlInterface.Name)
		if err != nil {
			return fmt.Errorf("failed to get peer interface %s: %w", imlInterface.Name, err)
		}
		// Set the peer interface's IP address
		if err := netlink.AddrAdd(containerLink, &netlink.Addr{IPNet: sid}); err != nil {
			return fmt.Errorf("failed to add IP address to peer interface %s: %w", imlInterface.Name, err)
		}

		encap := &netlink.Seg6LocalEncap{
			Action: netlink.SEG6_LOCAL_ACTION_END,
		}

		route := &netlink.Route{
			Dst:   dstNet,
			Gw:    gwIP,
			Src:   sid.IP,
			Scope: netlink.SCOPE_UNIVERSE,
			Encap: &netlink.SEG6Encap{

			},
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
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
			},
		},
		Routes: []*types.Route{
			{
				Dst: *dstNet,
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %s", resp.Status)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %s", resp.Status)
	}
	var configResponse NFConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &configResponse, nil
}

func tearDownNetworkFunction(netConfig *IMLCNIConfig, cniArgs *skel.CmdArgs) error {
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

func tearDownApplicationFunction(netConfig *IMLCNIConfig, cniArgs *skel.CmdArgs) error {
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
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("received non-200 response: %s", resp.Status)
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
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("received non-200 response: %s", resp.Status)
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
			if err := tearDownNetworkFunction(&cniConf, args); err != nil {
				logger.ErrorLogger().Printf("Failed to tear down network function: %v\n", err)
				return fmt.Errorf("failed to tear down network function: %w", err)
			}
		case "application_function":
			logger.InfoLogger().Printf("Tearing down application function for container %s\n", args.ContainerID)
			if err := tearDownApplicationFunction(&cniConf, args); err != nil {
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
