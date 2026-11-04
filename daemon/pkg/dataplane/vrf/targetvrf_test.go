package vrf

import (
	"slices"
	"testing"

	"github.com/vishvananda/netlink"
)

func fakeLink(name string, index, master int) netlink.Link {
	return &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name, Index: index, MasterIndex: master}}
}

func linkNames(links []netlink.Link) []string {
	names := make([]string, 0, len(links))
	for _, link := range links {
		names = append(names, link.Attrs().Name)
	}
	return names
}

func TestVRFDescendants(t *testing.T) {
	links := []netlink.Link{
		// Listed before their masters on purpose: ordering must come from the hierarchy, not the listing.
		fakeLink("member", 5, 4),
		fakeLink("bond", 4, 3),
		fakeLink("bridge", 3, 1),
		fakeLink("vrf", 1, 0),
		fakeLink("veth", 6, 1),
		fakeLink("other-vrf", 10, 0),
		fakeLink("other-slave", 11, 10),
		fakeLink("unenslaved", 12, 0),
	}

	got := vrfDescendants(1, links)
	names := linkNames(got)

	want := []string{"member", "bond", "bridge", "veth"}
	if len(names) != len(want) {
		t.Fatalf("vrfDescendants = %v, want the set %v", names, want)
	}
	for _, name := range want {
		if !slices.Contains(names, name) {
			t.Fatalf("vrfDescendants = %v, missing %s", names, name)
		}
	}
	// Each link must come before its master.
	pos := func(name string) int { return slices.Index(names, name) }
	if !(pos("member") < pos("bond") && pos("bond") < pos("bridge")) {
		t.Fatalf("vrfDescendants = %v, want each link before its master", names)
	}
}

func TestVRFDescendantsNoSlaves(t *testing.T) {
	links := []netlink.Link{fakeLink("vrf", 1, 0), fakeLink("eth0", 2, 0)}
	if got := vrfDescendants(1, links); len(got) != 0 {
		t.Fatalf("vrfDescendants = %v, want none", linkNames(got))
	}
}
