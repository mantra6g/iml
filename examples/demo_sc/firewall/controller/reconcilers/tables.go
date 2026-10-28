package reconcilers

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	FirewallSrcIPv4Table = "MyIngress.fw_src_ipv4"
	FirewallDstIPv4Table = "MyIngress.fw_dst_ipv4"
	FirewallSrcIPv6Table = "MyIngress.fw_src_ipv6"
	FirewallDstIPv6Table = "MyIngress.fw_dst_ipv6"
	AllowAction          = "MyIngress.allow"

	// MaxFirewallEntries matches FIREWALL_TABLE_SIZE in firewall.p4
	MaxFirewallEntries = 256
)

var tablesLog = ctrl.Log.WithName("tables")

// BuildFirewallTables returns the firewall table configuration that permits traffic from any subnet
// of the source Application to any subnet of the destination Application. A nil Application has no
// subnets, so its tables end up empty and every packet involving it is dropped.
func BuildFirewallTables(src, dst *corev1alpha1.Application) (map[string]corev1alpha1.TableConfig, error) {
	srcV4, srcV6 := collectSubnets(src)
	dstV4, dstV6 := collectSubnets(dst)

	tables := make(map[string]corev1alpha1.TableConfig, 4)
	for _, t := range []struct {
		name    string
		field   string
		subnets []netip.Prefix
	}{
		{FirewallSrcIPv4Table, "hdr.inner_ipv4.src_addr", srcV4},
		{FirewallDstIPv4Table, "hdr.inner_ipv4.dst_addr", dstV4},
		{FirewallSrcIPv6Table, "hdr.inner_ipv6.src_addr", srcV6},
		{FirewallDstIPv6Table, "hdr.inner_ipv6.dst_addr", dstV6},
	} {
		if len(t.subnets) > MaxFirewallEntries {
			return nil, fmt.Errorf("table %q: %d subnets exceed the table size of %d entries",
				t.name, len(t.subnets), MaxFirewallEntries)
		}
		tables[t.name] = corev1alpha1.TableConfig{Entries: buildEntries(t.field, t.subnets)}
	}
	return tables, nil
}

// buildEntries creates one allow entry per subnet, matching the field by longest prefix.
func buildEntries(field string, subnets []netip.Prefix) []corev1alpha1.TableEntry {
	entries := make([]corev1alpha1.TableEntry, 0, len(subnets))
	for _, subnet := range subnets {
		value := corev1alpha1.ParametrizedValue{}
		if subnet.Addr().Is4() {
			value.IPv4Address = new(subnet.Addr().String())
		} else {
			value.IPv6Address = new(subnet.Addr().String())
		}
		entries = append(entries, corev1alpha1.TableEntry{
			MatchFields: []corev1alpha1.MatchField{{
				Name: field,
				Type: corev1alpha1.LPMMatch,
				LPM: &corev1alpha1.LPMValue{
					Value:     value,
					PrefixLen: strconv.Itoa(subnet.Bits()),
				},
			}},
			Action: corev1alpha1.ActionConfig{
				Name: AllowAction,
			},
		})
	}
	return entries
}

// collectSubnets returns the deduplicated and sorted IPv4 and IPv6 subnets assigned to the Application
// across all nodes. Sorting keeps the generated entries stable, since Status.Subnets is a map.
func collectSubnets(app *corev1alpha1.Application) (v4, v6 []netip.Prefix) {
	if app == nil {
		return nil, nil
	}
	for node, subnets := range app.Status.Subnets {
		for _, subnet := range subnets {
			if prefix, ok := parseSubnet(subnet.IPv4Net, app.Name, node); ok && prefix.Addr().Is4() {
				v4 = append(v4, prefix)
			}
			if prefix, ok := parseSubnet(subnet.IPv6Net, app.Name, node); ok && prefix.Addr().Is6() {
				v6 = append(v6, prefix)
			}
		}
	}
	return sortAndCompact(v4), sortAndCompact(v6)
}

func parseSubnet(cidr, appName, node string) (netip.Prefix, bool) {
	if cidr == "" {
		return netip.Prefix{}, false
	}
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		tablesLog.Error(err, "Skipping invalid subnet", "application", appName, "node", node, "subnet", cidr)
		return netip.Prefix{}, false
	}
	return prefix.Masked(), true
}

func sortAndCompact(prefixes []netip.Prefix) []netip.Prefix {
	slices.SortFunc(prefixes, func(a, b netip.Prefix) int {
		if c := a.Addr().Compare(b.Addr()); c != 0 {
			return c
		}
		return a.Bits() - b.Bits()
	})
	return slices.Compact(prefixes)
}
