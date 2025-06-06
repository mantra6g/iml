package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
  cniConf := IMLCNIConfig{}
  if err := json.Unmarshal(cniArgs.StdinData, &cniConf); err != nil {
    return fmt.Errorf("failed to parse network config: %w", err)
  }

  var result types.Result
  switch cniConf.AppType {
    case "network_function":
      // Deploy the network function using the configuration
      result, err = deployNetworkFunction(cniArgs)
    case "application_function":
      hostname, err := os.Hostname()
      if err != nil {
        return fmt.Errorf("failed to get hostname: %w", err)
      }

      configRequest := IMLConfigRequest{
        ApplicationID: cniConf.AppId,
        HostID: hostname,
      }

      // Use the AppId to request the network configuration from IML
      netConfig, err := getConfigFromIML(configRequest)
      if err != nil {
        return fmt.Errorf("failed to get network config from IML: %w", err)
      }
      // Deploy the application function using the configuration
      result, err = deployApplicationFunction(netConfig, cniArgs)
    default:
      return fmt.Errorf("unknown app type: %s", cniConf.AppType)
  }
  if err != nil {
    return fmt.Errorf("failed to deploy network: %w", err)
  }

  return types.PrintResult(result, cniConf.CNIVersion)
}

func deployApplicationFunction(netConfig *IMLConfigResponse, cniArgs *skel.CmdArgs) (types.Result, error) {
  // Parse the IP address from the response
  _, ipNet, err := net.ParseCIDR(netConfig.IP)
  if err != nil {
    return nil, fmt.Errorf("failed to parse IP address: %s", netConfig.IP)
  }

  // Parse the MAC address from the response
  macAddr, err := net.ParseMAC(netConfig.MacAddress)
  if err != nil {
    return nil, fmt.Errorf("failed to parse MAC address: %s", netConfig.MacAddress)
  }

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

  // Parse the gateway MAC address from the response
  gwMac, err := net.ParseMAC(netConfig.Route.GatewayMac)
  if err != nil {
    return nil, fmt.Errorf("failed to parse gateway MAC: %s", netConfig.Route.GatewayMac)
  }

  // Open the container's network namespace
  ns, err := netns.GetFromPath(cniArgs.Netns)
  if err != nil {
    return nil, fmt.Errorf("failed to open netns %s: %w", cniArgs.Netns, err)
  }

  // Create a veth pair for the container
  // The container interface is called "iml0"
  // The host interface is called "veth-<containerID>"
  // The host interface will be added to the "vfbridge" bridge
  imlInterface := &netlink.Veth{
    LinkAttrs: netlink.LinkAttrs{
      Name: "veth-" + cniArgs.ContainerID[len(cniArgs.ContainerID)-6:],
      MTU: 1500,
      ParentDev: "vfbridge",
    },
    PeerName: "iml0",
    PeerMTU: 1500,
    PeerHardwareAddr: macAddr,
    PeerNamespace: ns,
  }

  // Add the veth pair
  if err := netlink.LinkAdd(imlInterface); err != nil {
    ns.Close()
    return nil, fmt.Errorf("failed to create veth pair: %w", err)
  }
  ns.Close()

  // Create route to the destination network
  routeLink := &netlink.Route{
    Dst:       dstNet,
    Gw:        gwIP,
    Src:       ipNet.IP,
  }

  // Create static ARP entry for the gateway MAC address
  arpEntry := &netlink.Neigh{
    IP:     gwIP,
    HardwareAddr: gwMac,
    State:  netlink.NUD_PERMANENT,
  }

  // Change to the container namespace
  err = execInsideNs(cniArgs.Netns, func() error {
    // Add the route inside the container's network namespace
    if err := netlink.RouteAdd(routeLink); err != nil {
      return fmt.Errorf("failed to add route: %w", err)
    }
    // Add the ARP entry inside the container's network namespace
    if err := netlink.NeighAdd(arpEntry); err != nil {
      return fmt.Errorf("failed to add ARP entry: %w", err)
    }
    // Get the peer interface of the veth pair
    peerLink, err := netlink.LinkByName(imlInterface.PeerName)
    if err != nil {
      return fmt.Errorf("failed to get peer interface %s: %w", imlInterface.PeerName, err)
    }
    // Set the peer interface's IP address
    if err := netlink.AddrAdd(peerLink, &netlink.Addr{IPNet: ipNet}); err != nil {
      return fmt.Errorf("failed to add IP address to peer interface %s: %w", imlInterface.PeerName, err)
    }
    // Disable arp on the peer interface
    if err := netlink.LinkSetARPOff(peerLink); err != nil {
      return fmt.Errorf("failed to disable ARP on peer interface %s: %w", imlInterface.PeerName, err)
    }
    return nil
  })
  if err != nil {
    return nil, fmt.Errorf("failed to execute inside netns %s: %w", cniArgs.Netns, err)
  }

  // Set both ends of the veth pair up
  if err := netlink.LinkSetUp(imlInterface); err != nil {
    return nil, fmt.Errorf("failed to set veth interface up: %w", err)
  }

  intfIndex := 0
  result := &current.Result{
    Interfaces: []*current.Interface{
      {
        Name:    imlInterface.PeerName,
        Mac:     netConfig.MacAddress,
        Sandbox: cniArgs.Netns,
      },
    },
    IPs: []*current.IPConfig{
      {
        Interface: &intfIndex,
        Address: *ipNet,
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

func deployNetworkFunction(cniArgs *skel.CmdArgs) (types.Result, error) {

  // Open the container's network namespace
  ns, err := netns.GetFromPath(cniArgs.Netns)
  if err != nil {
    return nil, fmt.Errorf("failed to open netns %s: %w", cniArgs.Netns, err)
  }

  // Create a veth pair for the container
  // The container interface is called "iml0"
  // The host interface is called "veth-<containerID>"
  // The host interface will be added to the "vfbridge" bridge
  imlInterface := &netlink.Veth{
    LinkAttrs: netlink.LinkAttrs{
      Name: "veth-" + cniArgs.ContainerID[len(cniArgs.ContainerID)-6:],
      MTU: 1500,
      ParentDev: "vfbridge",
    },
    PeerName: "iml0",
    PeerMTU: 1500,
    PeerNamespace: ns,
  }

  // Add the veth pair
  if err := netlink.LinkAdd(imlInterface); err != nil {
    ns.Close()
    return nil, fmt.Errorf("failed to create veth pair: %w", err)
  }

  // Parse the MAC address of the input interface
  macAddr, err := net.ParseMAC("00:11:22:33:44:55")
  if err != nil {
    ns.Close()
    return nil, fmt.Errorf("failed to parse MAC address: %w", err)
  }

  inputIntf := &netlink.Macvlan{
    LinkAttrs: netlink.LinkAttrs{
      Name: "in0",
      MTU: 1500,
      HardwareAddr: macAddr,
      ParentDev: "iml0",
      Namespace: ns,
    },
  }
  outputIntf := &netlink.Macvlan{
    LinkAttrs: netlink.LinkAttrs{
      Name: "out0",
      MTU: 1500,
      ParentDev: "iml0",
      Namespace: ns,
    },
  }

  // Add the input interface
  if err := netlink.LinkAdd(inputIntf); err != nil {
    ns.Close()
    return nil, fmt.Errorf("failed to create input interface: %w", err)
  }

  // Add the output interface
  if err := netlink.LinkAdd(outputIntf); err != nil {
    ns.Close()
    return nil, fmt.Errorf("failed to create output interface: %w", err)
  }
  ns.Close()

  result := &current.Result{
    Interfaces: []*current.Interface{
      {
        Name:    imlInterface.PeerName,
        Mac:     imlInterface.HardwareAddr.String(),
        Sandbox: cniArgs.Netns,
      },
      {
        Name:    inputIntf.Name,
        Mac:     inputIntf.HardwareAddr.String(),
        Sandbox: cniArgs.Netns,
      },
      {
        Name:    outputIntf.Name,
        Sandbox: cniArgs.Netns,
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

func getConfigFromIML(payload IMLConfigRequest) (*IMLConfigResponse, error) {
  data, err := json.Marshal(payload)
  if err != nil {
    return nil, fmt.Errorf("failed to marshal request payload: %w", err)
  }

  resp, err := http.Post(
    "http://iml.desire6g-system.svc.cluster.local/iml/cni/register", 
    "application/json", bytes.NewBuffer(data),
  )
  if err != nil {
    return nil, fmt.Errorf("failed to make HTTP request: %w", err)
  }
  defer resp.Body.Close()

  if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("received non-200 response: %s", resp.Status)
  }
  var configResponse IMLConfigResponse
  if err := json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
    return nil, fmt.Errorf("failed to decode response: %w", err)
  }

  return &configResponse, nil
}

func cmdDel(args *skel.CmdArgs) error {
  fmt.Fprintf(os.Stderr, "DEL called for container %s\n", args.ContainerID)
  return nil
}

func cmdCheck(args *skel.CmdArgs) error {
  fmt.Fprintf(os.Stderr, "CHECK not implemented\n")
  return nil
}

func versionInfo() version.PluginInfo {
  return version.PluginSupports("0.3.1","0.4.0")
}

func main() {
  skel.PluginMainFuncs(skel.CNIFuncs{
    Add: cmdAdd,
    Del: cmdDel,
    Check: cmdCheck,
  }, versionInfo(), "CNI Plugin for IML")
}