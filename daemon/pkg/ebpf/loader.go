//go:build linux

package ebpf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const (
	// DefaultSharedProgramsPinPath is the bpffs directory where programs shared by every target are pinned.
	DefaultSharedProgramsPinPath = "/sys/fs/bpf/loom/programs"

	// DefaultSharedMapsPinPath is the bpffs directory where maps shared by every target are pinned.
	DefaultSharedMapsPinPath = "/sys/fs/bpf/loom/maps"

	// DefaultTargetPinPathFormat is the bpffs directory holding every object pinned for a specific target.
	DefaultTargetPinPathFormat = "/sys/fs/bpf/loom/targets/%s"

	// DefaultTargetProgramsPinPathFormat is the bpffs directory where programs for a specific target are pinned.
	DefaultTargetProgramsPinPathFormat = DefaultTargetPinPathFormat + "/programs"

	// DefaultTargetMapsPinPathFormat is the bpffs directory where maps for a specific target are pinned.
	DefaultTargetMapsPinPathFormat = DefaultTargetPinPathFormat + "/maps"
)

// PinPaths are the bpffs directories programs and maps are pinned under. An empty field
// disables pinning for that kind of object.
type PinPaths struct {
	Programs string
	Maps     string
}

// SharedPinPaths returns the pin paths of objects shared by every target, such as srv6_end.
func SharedPinPaths() PinPaths {
	return PinPaths{Programs: DefaultSharedProgramsPinPath, Maps: DefaultSharedMapsPinPath}
}

// TargetPinPaths returns the pin paths of the objects owned by a single target, such as its
// srv6_dispatch instance.
func TargetPinPaths(target string) PinPaths {
	return PinPaths{
		Programs: fmt.Sprintf(DefaultTargetProgramsPinPathFormat, target),
		Maps:     fmt.Sprintf(DefaultTargetMapsPinPathFormat, target),
	}
}

// UnpinTarget removes every object pinned for target. Programs still attached to an interface
// stay loaded until they are detached. It is a no-op if nothing is pinned.
func UnpinTarget(target string) error {
	dir := fmt.Sprintf(DefaultTargetPinPathFormat, target)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove pin path %q: %w", dir, err)
	}
	return nil
}

// Direction selects the TC hook a program is attached to.
type Direction int

const (
	Ingress Direction = iota
	Egress
)

// LoadSRv6End loads the SRv6 End program into the kernel. It is shared by every target, so
// pass SharedPinPaths: the first call pins it, later calls reuse the pinned program. The
// caller owns the returned objects and must Close them; pinned programs, and programs
// attached with AttachTC, stay loaded afterwards.
func LoadSRv6End(pins PinPaths, opts *ebpf.CollectionOptions) (*Srv6EndObjects, error) {
	spec, err := LoadSrv6End()
	if err != nil {
		return nil, fmt.Errorf("failed to load srv6_end spec: %w", err)
	}
	objs := &Srv6EndObjects{}
	if err := load(spec, pins, opts, objs); err != nil {
		return nil, fmt.Errorf("failed to load srv6_end: %w", err)
	}
	if err := pinOrReuse(&objs.Srv6End, pins.Programs, "srv6_end"); err != nil {
		objs.Close()
		return nil, err
	}
	return objs, nil
}

// LoadSRv6Dispatch loads a target's flow dispatcher into the kernel. Each target has its own
// instance, so pass TargetPinPaths(target): the first call pins the program and its "cfg" and
// "buckets" maps, later calls (e.g. after a daemon restart) reuse them. The caller owns the returned
// objects and must Close them.
func LoadSRv6Dispatch(pins PinPaths, opts *ebpf.CollectionOptions) (*Srv6DispatchObjects, error) {
	spec, err := LoadSrv6Dispatch()
	if err != nil {
		return nil, fmt.Errorf("failed to load srv6_dispatch spec: %w", err)
	}
	objs := &Srv6DispatchObjects{}
	if err := load(spec, pins, opts, objs); err != nil {
		return nil, fmt.Errorf("failed to load srv6_dispatch: %w", err)
	}
	if err := pinOrReuse(&objs.Srv6Dispatch, pins.Programs, "srv6_dispatch"); err != nil {
		objs.Close()
		return nil, err
	}
	return objs, nil
}

// load loads spec into objs, pinning its maps by name under pins.Maps (or reusing the ones
// already pinned there).
func load(spec *ebpf.CollectionSpec, pins PinPaths, opts *ebpf.CollectionOptions, objs any) error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("failed to remove memlock rlimit: %w", err)
	}
	var o ebpf.CollectionOptions
	if opts != nil {
		o = *opts
	}
	// The C sources don't mark maps for pinning, so where they go is decided here.
	for _, m := range spec.Maps {
		m.Pinning = ebpf.PinNone
		if pins.Maps != "" {
			m.Pinning = ebpf.PinByName
		}
	}
	if pins.Maps != "" && len(spec.Maps) > 0 {
		if err := os.MkdirAll(pins.Maps, 0o700); err != nil {
			return fmt.Errorf("failed to create pin path %q: %w", pins.Maps, err)
		}
		o.Maps.PinPath = pins.Maps
	}
	if err := spec.LoadAndAssign(objs, &o); err != nil {
		return verifierDetails(err)
	}
	return nil
}

// pinOrReuse pins *prog as dir/name. If a program is already pinned there, *prog is closed
// and replaced by the pinned one, so every caller shares a single instance. An empty dir
// disables pinning.
func pinOrReuse(prog **ebpf.Program, dir, name string) error {
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, name)
	pinned, err := ebpf.LoadPinnedProgram(path, nil)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create pin path %q: %w", dir, err)
		}
		err = (*prog).Pin(path)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("failed to pin %s to %q: %w", name, path, err)
		}
		// Another invocation pinned it in the meantime.
		pinned, err = ebpf.LoadPinnedProgram(path, nil)
	}
	if err != nil {
		return fmt.Errorf("failed to load pinned %s from %q: %w", name, path, err)
	}
	(*prog).Close()
	*prog = pinned
	return nil
}

// AttachTC attaches prog as a direct-action filter on the clsact qdisc of ifaceName. It is
// idempotent: calling it again replaces the existing qdisc and filter. The filter holds a
// reference to the program, so it stays attached after the calling process exits.
func AttachTC(prog *ebpf.Program, ifaceName string, dir Direction) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to lookup interface %q: %w", ifaceName, err)
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err = netlink.QdiscReplace(qdisc); err != nil {
		return fmt.Errorf("failed to set up clsact qdisc on %q: %w", ifaceName, err)
	}

	parent := uint32(netlink.HANDLE_MIN_INGRESS)
	if dir == Egress {
		parent = netlink.HANDLE_MIN_EGRESS
	}
	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    parent,
			Handle:    1,
			Protocol:  unix.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           prog.FD(),
		Name:         programName(prog),
		DirectAction: true,
	}
	if err = netlink.FilterReplace(filter); err != nil {
		return fmt.Errorf("failed to attach %s to %q: %w", filter.Name, ifaceName, err)
	}
	return nil
}

// DetachTC removes the filter AttachTC installed on ifaceName, leaving the clsact qdisc and any
// other filter in place. It is a no-op if the interface, the qdisc or the filter is missing.
func DetachTC(ifaceName string, dir Direction) error {
	link, err := netlink.LinkByName(ifaceName)
	if errors.As(err, &netlink.LinkNotFoundError{}) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to lookup interface %q: %w", ifaceName, err)
	}

	qdiscs, err := netlink.QdiscList(link)
	if err != nil {
		return fmt.Errorf("failed to list qdiscs of %q: %w", ifaceName, err)
	}
	hasClsact := false
	for _, q := range qdiscs {
		if q.Type() == "clsact" {
			hasClsact = true
			break
		}
	}
	if !hasClsact {
		return nil
	}

	parent := uint32(netlink.HANDLE_MIN_INGRESS)
	if dir == Egress {
		parent = netlink.HANDLE_MIN_EGRESS
	}
	filters, err := netlink.FilterList(link, parent)
	if err != nil {
		return fmt.Errorf("failed to list filters of %q: %w", ifaceName, err)
	}
	for _, f := range filters {
		bpf, ok := f.(*netlink.BpfFilter)
		if !ok || bpf.Handle != 1 || bpf.Priority != 1 {
			continue
		}
		if err = netlink.FilterDel(bpf); err != nil {
			return fmt.Errorf("failed to detach %s from %q: %w", bpf.Name, ifaceName, err)
		}
	}
	return nil
}

func programName(prog *ebpf.Program) string {
	if info, err := prog.Info(); err == nil && info.Name != "" {
		return info.Name
	}
	return "iml"
}

// verifierDetails replaces a verifier rejection with the full verifier log, which is
// otherwise truncated to its last line.
func verifierDetails(err error) error {
	var verr *ebpf.VerifierError
	if errors.As(err, &verr) {
		return fmt.Errorf("%w\n%+v", err, verr)
	}
	return err
}
