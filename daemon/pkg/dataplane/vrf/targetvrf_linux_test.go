package vrf

import (
	"net"
	"os"
	"runtime"
	"testing"

	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

// inNewNetns runs f in a fresh network namespace, so the test never touches the host's interfaces.
// It needs CAP_NET_ADMIN, e.g. run as root or under `unshare -rn`.
func inNewNetns(t *testing.T, f func()) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (or `unshare -rn go test ...`) to create network namespaces")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	origin, err := netns.Get()
	if err != nil {
		t.Fatalf("failed to get current netns: %v", err)
	}
	defer origin.Close()
	ns, err := netns.New()
	if err != nil {
		t.Skipf("cannot create network namespace: %v", err)
	}
	defer ns.Close()
	defer func() {
		if err := netns.Set(origin); err != nil {
			t.Fatalf("failed to restore netns: %v", err)
		}
	}()
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		t.Fatalf("failed to get lo: %v", err)
	}
	if err = netlink.LinkSetUp(lo); err != nil {
		t.Fatalf("failed to set lo up: %v", err)
	}
	f()
}

func mustParseCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", s, err)
	}
	ipNet.IP = ip
	return ipNet
}

func tableRoutes(t *testing.T, table int) []netlink.Route {
	t.Helper()
	var all []netlink.Route
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
		if err != nil {
			t.Fatalf("failed to list routes of table %d: %v", table, err)
		}
		all = append(all, routes...)
	}
	return all
}

func hasRoute(routes []netlink.Route, dst string, routeType int) bool {
	for _, r := range routes {
		if r.Dst != nil && r.Dst.String() == dst && r.Type == routeType {
			return true
		}
	}
	return false
}

func TestTargetVRFLifecycle(t *testing.T) {
	inNewNetns(t, func() {
		const table = 4242
		nfCIDR := netutils.DualStackNetwork{IPv6Net: mustParseCIDR(t, "fd00:aa::/64")}

		targetVRF, err := NewTargetVRF(logr.Discard(), table, nfCIDR)
		if err != nil {
			t.Fatalf("NewTargetVRF: %v", err)
		}
		vrfLink, err := netlink.LinkByName(TargetVRFName(table))
		if err != nil {
			t.Fatalf("VRF not created: %v", err)
		}
		if got := vrfLink.(*netlink.Vrf).Table; got != table {
			t.Fatalf("VRF table = %d, want %d", got, table)
		}
		if !hasRoute(tableRoutes(t, table), "fd00:aa::/64", unix.RTN_UNREACHABLE) {
			t.Fatalf("unreachable NF CIDR route missing from table %d: %v", table, tableRoutes(t, table))
		}

		// Stand-in for the veth pair created by createSubnetToSubnetTunnels.
		veth := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: "tvtest-in"}, PeerName: "tvtest-out"}
		if err = netlink.LinkAdd(veth); err != nil {
			t.Fatalf("failed to add veth: %v", err)
		}
		if err = netlink.LinkSetMaster(veth, vrfLink); err != nil {
			t.Fatalf("failed to enslave veth: %v", err)
		}
		if err = netlink.AddrAdd(veth, &netlink.Addr{IPNet: mustParseCIDR(t, "fd01::1/126")}); err != nil {
			t.Fatalf("failed to add veth address: %v", err)
		}
		if err = netlink.LinkSetUp(veth); err != nil {
			t.Fatalf("failed to set veth up: %v", err)
		}
		peer, err := netlink.LinkByName("tvtest-out")
		if err != nil {
			t.Fatalf("failed to get veth peer: %v", err)
		}
		if err = netlink.LinkSetUp(peer); err != nil {
			t.Fatalf("failed to set veth peer up: %v", err)
		}
		targetVRF.SetTunnel("tvtest-in")
		targetVRF.RouterTunnelAddrs = netutils.DualStackAddress{IPv6: net.ParseIP("fd01::2")}
		if err = targetVRF.AddDefaultRoute(); err != nil {
			t.Fatalf("AddDefaultRoute: %v", err)
		}
		if !hasRoute(tableRoutes(t, table), "::/0", unix.RTN_UNICAST) {
			t.Fatalf("default route missing from table %d: %v", table, tableRoutes(t, table))
		}

		// What a plugin would do: enslave its own interface, route through it, and add a device-less route.
		pluginIface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "tvtest-plugin", MasterIndex: vrfLink.Attrs().Index}}
		if err = netlink.LinkAdd(pluginIface); err != nil {
			t.Fatalf("failed to add plugin interface: %v", err)
		}
		if err = netlink.LinkSetUp(pluginIface); err != nil {
			t.Fatalf("failed to set plugin interface up: %v", err)
		}
		pluginLink, err := netlink.LinkByName("tvtest-plugin")
		if err != nil {
			t.Fatalf("failed to get plugin interface: %v", err)
		}
		if err = netlink.RouteAdd(&netlink.Route{
			Dst: mustParseCIDR(t, "fd00:bb::/64"), Table: table, LinkIndex: pluginLink.Attrs().Index,
		}); err != nil {
			t.Fatalf("failed to add plugin route: %v", err)
		}
		if err = netlink.RouteAdd(&netlink.Route{
			Dst: mustParseCIDR(t, "fd00:cc::/64"), Table: table, Type: unix.RTN_BLACKHOLE,
		}); err != nil {
			t.Fatalf("failed to add device-less plugin route: %v", err)
		}

		// Interfaces wired to the VRF through another master: a bridge inside the VRF, with a dummy port
		// and a veth port whose peer stays outside the VRF.
		bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: "tvtest-br", MasterIndex: vrfLink.Attrs().Index}}
		if err = netlink.LinkAdd(bridge); err != nil {
			t.Fatalf("failed to add bridge: %v", err)
		}
		bridgeLink, err := netlink.LinkByName("tvtest-br")
		if err != nil {
			t.Fatalf("failed to get bridge: %v", err)
		}
		bridgePort := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "tvtest-brport", MasterIndex: bridgeLink.Attrs().Index}}
		if err = netlink.LinkAdd(bridgePort); err != nil {
			t.Fatalf("failed to add bridge port: %v", err)
		}
		bridgeVeth := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: "tvtest-brveth"}, PeerName: "tvtest-brpeer"}
		if err = netlink.LinkAdd(bridgeVeth); err != nil {
			t.Fatalf("failed to add bridge veth: %v", err)
		}
		if err = netlink.LinkSetMaster(bridgeVeth, bridgeLink); err != nil {
			t.Fatalf("failed to attach veth to bridge: %v", err)
		}

		targetVRF.Teardown()

		for _, name := range []string{
			TargetVRFName(table), "tvtest-in", "tvtest-out", "tvtest-plugin",
			"tvtest-br", "tvtest-brport", "tvtest-brveth", "tvtest-brpeer",
		} {
			if _, err := netlink.LinkByName(name); err == nil {
				t.Errorf("interface %s still exists after teardown", name)
			}
		}
		if routes := tableRoutes(t, table); len(routes) != 0 {
			t.Errorf("table %d not flushed after teardown: %v", table, routes)
		}
	})
}

func TestNewTargetVRFReplacesStaleVRF(t *testing.T) {
	inNewNetns(t, func() {
		const table = 4343
		nfCIDR := netutils.DualStackNetwork{IPv6Net: mustParseCIDR(t, "fd00:dd::/64")}
		if _, err := NewTargetVRF(logr.Discard(), table, nfCIDR); err != nil {
			t.Fatalf("first NewTargetVRF: %v", err)
		}
		// Simulates a daemon restart without teardown: the VRF from the previous run is still there.
		targetVRF, err := NewTargetVRF(logr.Discard(), table, nfCIDR)
		if err != nil {
			t.Fatalf("NewTargetVRF over a stale VRF: %v", err)
		}
		defer targetVRF.Teardown()
		if !hasRoute(tableRoutes(t, table), "fd00:dd::/64", unix.RTN_UNREACHABLE) {
			t.Fatalf("unreachable NF CIDR route missing after re-creation")
		}
	})
}
