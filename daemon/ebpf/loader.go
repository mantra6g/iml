package ebpf

import (
	"bytes"
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	_ "embed"
)

type Program []byte

// SRv6EndProgram is the bytecode for the eBPF program that applies the SRv6 End behavior to packets.
//
//go:embed srv6_end.ebpf.o
var SRv6EndProgram Program

// SRv6SR0Program is the bytecode for the eBPF program that steers traffic to p4targets with
// split input and output interfaces, i.e., an interface in0 that only receives packets and
// an interface out0 that only sends packets.
//
//go:embed srv6_splitio.ebpf.o
var SRv6SR0Program Program

func AttachProgramToInterface(program Program, ifaceName string) error {
	// 1. Load the BPF program into the kernel
	// This takes the embedded bytes and returns a kernel object
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(program))
	if err != nil {
		return fmt.Errorf("failed to load spec: %v", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("failed to create collection: %v", err)
	}
	defer coll.Close()

	// "tc" matches the function name in the C code
	prog, ok := coll.Programs["force_recalc_srv6_csum"]
	if !ok {
		return fmt.Errorf("failed to find tc program")
	}

	// 2. Find the link
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to lookup interface %q: %v", ifaceName, err)
	}

	// 3. Setup Qdisc (clsact)
	// Note: We don't need to specify a Parent for clsact usually
	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0), // ffff:
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err = netlink.QdiscAdd(qdisc); err != nil {
		return fmt.Errorf("Qdisc for %s already exists: %v\n", ifaceName, err)
	}

	// 4. Use the updated BpfFilter struct
	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Handle:    1,
			Protocol:  unix.ETH_P_ALL, // ETH_P_ALL
			Priority:  1,
		},
		Fd:           prog.FD(), // This is the key change!
		Name:         "recalc_csum",
		DirectAction: true,
	}
	if err = netlink.FilterAdd(filter); err != nil {
		return fmt.Errorf("failed to attach filter to %s: %v", err, ifaceName)
	}
	return nil
}
