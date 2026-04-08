package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"imlcni/logger"
	"net/http"
	"os"

	nsutils "imlcni/utils/netns"

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

	args := os.Getenv("CNI_ARGS")
	k8sArgs := &K8sArgs{}
	if err := types.LoadArgs(args, k8sArgs); err != nil {
		return err
	}

	var result types.Result
	switch cniConf.Args.CNI.AppType {
	case P4TargetType:
		logger.InfoLogger().Printf("Deploying programmable target config for container %s\n", cniArgs.ContainerID)

		configRequest := ContainerizedP4TargetConfigRequest{
			ContainerID:  cniArgs.ContainerID,
			P4TargetName: cniConf.Args.CNI.TargetName,
		}

		// Use the NFId to request the network configuration from IML
		netConfig, err := getP4TargetConfigFromIML(configRequest)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to get programmable target config from IML: %v\n", err)
			return fmt.Errorf("failed to get programmable target config from IML: %w", err)
		}
		err = nsutils.EnableSRv6InNamespace(cniArgs.Netns)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to enable SRv6 in namespace: %v\n", err)
		}
		result, err = DeployNetworkConfiguration(netConfig, cniArgs)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to deploy programmable target: %v\n", err)
			return fmt.Errorf("failed to deploy programmable target: %w", err)
		}

	case ApplicationType:
		logger.InfoLogger().Printf("Deploying application function for container %s\n", cniArgs.ContainerID)

		configRequest := AppInstanceConfigRequest{
			ContainerID:  cniArgs.ContainerID,
			PodName:      string(k8sArgs.PodName),
			PodNamespace: string(k8sArgs.PodNamespace),
			AppName:      cniConf.Args.CNI.AppName,
			AppNamespace: cniConf.Args.CNI.AppNamespace,
		}

		// Use the AppId to request the network configuration from IML
		netConfig, err := getAppConfigFromIML(configRequest)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to get network config from IML: %v\n", err)
			return fmt.Errorf("failed to get network config from IML: %w", err)
		}
		result, err = DeployNetworkConfiguration(netConfig, cniArgs)
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

func DeployNetworkConfiguration(netConfig *NetworkConfig, cniArgs *skel.CmdArgs) (types.Result, error) {
	logger.DebugLogger().Printf("Deploying application with config: %+v\n", netConfig)

	if netConfig.IPNets.IsEmpty() {
		return nil, fmt.Errorf("IPNets is empty")
	}
	if netConfig.Gateways.IsEmpty() {
		return nil, fmt.Errorf("gateways is empty")
	}
	if netConfig.ClusterCIDRs.IsEmpty() {
		return nil, fmt.Errorf("clusterCIDRs is empty")
	}

	ipv4Enabled := netConfig.IPNets.IPv4Net != nil
	ipv6Enabled := netConfig.IPNets.IPv6Net != nil
	if ipv4Enabled && (netConfig.Gateways.IPv4 == nil || netConfig.ClusterCIDRs.IPv4Net == nil) {
		return nil, fmt.Errorf("inconsistent IPv4 configuration: IPNet is set but gateway or cluster CIDR is not")
	}
	if ipv6Enabled && (netConfig.Gateways.IPv6 == nil || netConfig.ClusterCIDRs.IPv6Net == nil) {
		return nil, fmt.Errorf("inconsistent IPv6 configuration: IPNet is set but gateway or cluster CIDR is not")
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
			MTU:  int(netConfig.MTU),
		},
		PeerName: netConfig.IfaceName,
		PeerMTU:  netConfig.MTU,
	}

	// Change to the container namespace
	err = nsutils.ExecInsideNetworkNamespace(cniArgs.Netns, func() error {
		// Add the veth pair
		if err = netlink.LinkAdd(imlInterface); err != nil {
			return fmt.Errorf("failed to create veth pair: %w", err)
		}
		var peerInterface netlink.Link
		peerInterface, err = netlink.LinkByName(imlInterface.PeerName)
		if err != nil {
			return fmt.Errorf("failed to get host interface %s: %w", imlInterface.PeerName, err)
		}
		// Move the peer interface to the host namespace
		if err = netlink.LinkSetNsFd(peerInterface, int(hostNs)); err != nil {
			return fmt.Errorf("failed to move host interface %s to host netns: %w", imlInterface.PeerName, err)
		}
		// Set both ends of the veth pair up
		if err = netlink.LinkSetUp(imlInterface); err != nil {
			return fmt.Errorf("failed to set container interface up: %w", err)
		}
		// Set the container interface's IP addresses
		if ipv4Enabled {
			if err = netlink.AddrAdd(imlInterface, &netlink.Addr{IPNet: netConfig.IPNets.IPv4Net}); err != nil {
				return fmt.Errorf("failed to add IPv4 address to container interface %s: %w", imlInterface.Name, err)
			}
		}
		if ipv6Enabled {
			if err = netlink.AddrAdd(imlInterface, &netlink.Addr{IPNet: netConfig.IPNets.IPv6Net}); err != nil {
				return fmt.Errorf("failed to add IPv6 address to container interface %s: %w", imlInterface.Name, err)
			}
		}

		// Create route to the destination network
		if ipv4Enabled {
			routeLink := &netlink.Route{
				Dst:   netConfig.ClusterCIDRs.IPv4Net,
				Gw:    netConfig.Gateways.IPv4,
				Scope: netlink.SCOPE_UNIVERSE,
			}
			if err = netlink.RouteAdd(routeLink); err != nil {
				return fmt.Errorf("failed to add route: %w", err)
			}
		}
		if ipv6Enabled {
			routeLink := &netlink.Route{
				Dst:   netConfig.ClusterCIDRs.IPv6Net,
				Gw:    netConfig.Gateways.IPv6,
				Scope: netlink.SCOPE_UNIVERSE,
			}
			if err = netlink.RouteAdd(routeLink); err != nil {
				return fmt.Errorf("failed to add route: %w", err)
			}
		}
		return nil
	})
	_ = hostNs.Close()
	if errors.Is(err, &nsutils.FunctionExecutionError{}) {
		return nil, fmt.Errorf("failed to execute inside container netns %s: %w", cniArgs.Netns, err)
	} else if err != nil {
		return nil, err
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
				Mtu:     int(netConfig.MTU),
			},
		},
		IPs:    make([]*current.IPConfig, 0),
		Routes: make([]*types.Route, 0),
	}
	if ipv4Enabled {
		result.IPs = append(result.IPs, &current.IPConfig{
			Interface: &intfIndex,
			Address:   *netConfig.IPNets.IPv4Net,
			Gateway:   netConfig.Gateways.IPv4,
		})
	}
	if ipv6Enabled {
		result.IPs = append(result.IPs, &current.IPConfig{
			Interface: &intfIndex,
			Address:   *netConfig.IPNets.IPv6Net,
			Gateway:   netConfig.Gateways.IPv6,
		})
	}
	return result, nil
}

func getAppConfigFromIML(payload AppInstanceConfigRequest) (*NetworkConfig, error) {
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
	var configResponse NetworkConfig
	if err = json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &configResponse, nil
}

func getP4TargetConfigFromIML(configRequest ContainerizedP4TargetConfigRequest) (*NetworkConfig, error) {
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
	var configResponse NetworkConfig
	if err = json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &configResponse, nil
}

func tearDownP4Target(cniArgs *skel.CmdArgs) error {
	logger.InfoLogger().Printf("Tearing down p4target for container %s in netns %s\n", cniArgs.ContainerID, cniArgs.Netns)

	// Notify IML of the network function teardown
	configRequest := ContainerizedP4TargetTeardownRequest{
		ContainerID: cniArgs.ContainerID,
	}
	err := notifyIMLOfP4TargetTeardown(configRequest)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to notify IML of teardown: %v\n", err)
		return fmt.Errorf("failed to notify IML of teardown: %w", err)
	}

	// Change to the container namespace
	err = nsutils.ExecInsideNetworkNamespace(cniArgs.Netns, func() error {
		// Get the peer interface by name inside the container's namespace
		intf, err := netlink.LinkByName("iml0")
		if err != nil {
			return fmt.Errorf("failed to get interface %s in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		// Set the interface down
		if err = netlink.LinkSetDown(intf); err != nil {
			return fmt.Errorf("failed to set interface %s down in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		// Delete the interface
		if err = netlink.LinkDel(intf); err != nil {
			return fmt.Errorf("failed to delete interface %s in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		return nil
	})
	if errors.Is(err, &nsutils.FunctionExecutionError{}) {
		logger.ErrorLogger().Printf("Failed to execute inside netns %s: %v\n", cniArgs.Netns, err)
		return fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
	}
	return err
}

func tearDownApplication(cniArgs *skel.CmdArgs) error {
	logger.InfoLogger().Printf("Tearing down application function for container %s in netns %s\n", cniArgs.ContainerID, cniArgs.Netns)

	// Notify IML of the application function teardown
	configRequest := AppInstanceTeardownRequest{
		ContainerID: cniArgs.ContainerID,
	}
	err := notifyIMLOfAppTeardown(configRequest)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to notify IML of teardown: %v\n", err)
		return fmt.Errorf("failed to notify IML of teardown: %w", err)
	}

	// Change to the container namespace
	err = nsutils.ExecInsideNetworkNamespace(cniArgs.Netns, func() error {
		// Get the peer interface by name inside the container's namespace
		intf, err := netlink.LinkByName("iml0")
		if err != nil {
			// If the interface does not exist, consider it already torn down
			return nil
		}

		// Set the interface down
		if err = netlink.LinkSetDown(intf); err != nil {
			return fmt.Errorf("failed to set interface %s down in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		// Delete the interface
		if err = netlink.LinkDel(intf); err != nil {
			return fmt.Errorf("failed to delete interface %s in netns %s: %w", "iml0", cniArgs.Netns, err)
		}

		return nil
	})
	if errors.Is(err, &nsutils.FunctionExecutionError{}) {
		logger.ErrorLogger().Printf("Failed to execute inside netns %s: %v\n", cniArgs.Netns, err)
		return fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
	}
	return err
}

func notifyIMLOfAppTeardown(request AppInstanceTeardownRequest) error {
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

func notifyIMLOfP4TargetTeardown(request ContainerizedP4TargetTeardownRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %w", err)
	}

	resp, err := http.Post(
		"http://localhost:7623/api/v1/cni/p4target/teardown",
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
		if err := tearDownP4Target(args); err != nil {
			logger.ErrorLogger().Printf("Failed to tear down network function: %v\n", err)
			return fmt.Errorf("failed to tear down network function: %w", err)
		}
	case "application_function":
		logger.InfoLogger().Printf("Tearing down application function for container %s\n", args.ContainerID)
		if err := tearDownApplication(args); err != nil {
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
