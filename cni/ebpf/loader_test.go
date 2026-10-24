//go:build linux

package ebpf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// TC verdicts from linux/pkt_cls.h.
const (
	tcActOK       = 0
	tcActShot     = 2
	tcActRedirect = 7
)

// requirePrivileges skips tests that load programs into the kernel when the process lacks
// CAP_BPF (non-root, or root in an unprivileged container). Run them with `make ebpf-verify`.
func requirePrivileges(t *testing.T) {
	t.Helper()
	const hint = "run `make ebpf-verify`"
	if err := rlimit.RemoveMemlock(); err != nil {
		t.Skipf("cannot remove memlock rlimit (%v), %s", err, hint)
	}
	m, err := ebpf.NewMap(&ebpf.MapSpec{Type: ebpf.Array, KeySize: 4, ValueSize: 4, MaxEntries: 1})
	if errors.Is(err, unix.EPERM) {
		t.Skipf("not allowed to create eBPF maps, %s", hint)
	}
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
}

func TestVerifier(t *testing.T) {
	requirePrivileges(t)
	opts := &ebpf.CollectionOptions{Programs: ebpf.ProgramOptions{LogLevel: ebpf.LogLevelInstruction}}

	t.Run("srv6_end", func(t *testing.T) {
		objs, err := LoadSRv6End(PinPaths{}, opts)
		if err != nil {
			t.Fatal(err)
		}
		objs.Close()
	})
	t.Run("srv6_dispatch", func(t *testing.T) {
		objs, err := LoadSRv6Dispatch(PinPaths{}, opts)
		if err != nil {
			t.Fatal(err)
		}
		objs.Close()
	})
}

func TestPinnedObjectsAreShared(t *testing.T) {
	requirePrivileges(t)
	root := filepath.Join("/sys/fs/bpf", fmt.Sprintf("iml-test-%d", os.Getpid()))
	t.Cleanup(func() { os.RemoveAll(root) })
	pins := PinPaths{Programs: filepath.Join(root, "programs"), Maps: filepath.Join(root, "maps")}

	sameProgram := func(t *testing.T, a, b *ebpf.Program) {
		t.Helper()
		ia, err := a.Info()
		if err != nil {
			t.Fatal(err)
		}
		ib, err := b.Info()
		if err != nil {
			t.Fatal(err)
		}
		idA, _ := ia.ID()
		idB, _ := ib.ID()
		if idA != idB {
			t.Fatalf("second load got program %d, want the pinned program %d", idB, idA)
		}
	}

	t.Run("srv6_end", func(t *testing.T) {
		first, err := LoadSRv6End(pins, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		second, err := LoadSRv6End(pins, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Close()
		sameProgram(t, first.Srv6End, second.Srv6End)
	})
	t.Run("srv6_dispatch", func(t *testing.T) {
		first, err := LoadSRv6Dispatch(pins, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		second, err := LoadSRv6Dispatch(pins, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Close()
		sameProgram(t, first.Srv6Dispatch, second.Srv6Dispatch)

		want := Srv6DispatchConfig{DispatchIfindex: 7, Vmac: [6]byte{0x02, 0xaa, 0, 0, 0, 1}}
		if err := first.Cfg.Put(uint32(0), want); err != nil {
			t.Fatal(err)
		}
		var got Srv6DispatchConfig
		if err := second.Cfg.Lookup(uint32(0), &got); err != nil {
			t.Fatal(err)
		}
		if got.DispatchIfindex != want.DispatchIfindex || got.Vmac != want.Vmac {
			t.Fatalf("second load sees %+v, want %+v", got, want)
		}
	})
}

func TestAttachTC_IsIdempotent(t *testing.T) {
	requirePrivileges(t)
	objs, err := LoadSRv6End(PinPaths{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer objs.Close()

	dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: fmt.Sprintf("imltest%d", os.Getpid()%10000)}}
	if err := netlink.LinkAdd(dummy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { netlink.LinkDel(dummy) })

	for _, dir := range []Direction{Ingress, Ingress, Egress} {
		if err := AttachTC(objs.Srv6End, dummy.Name, dir); err != nil {
			t.Fatal(err)
		}
	}
	for parent, name := range map[uint32]string{netlink.HANDLE_MIN_INGRESS: "ingress", netlink.HANDLE_MIN_EGRESS: "egress"} {
		filters, err := netlink.FilterList(dummy, parent)
		if err != nil {
			t.Fatal(err)
		}
		if len(filters) != 1 {
			t.Fatalf("%d %s filters, want 1", len(filters), name)
		}
		if bpf, ok := filters[0].(*netlink.BpfFilter); !ok || !bpf.DirectAction {
			t.Fatalf("%s filter = %+v, want a direct-action bpf filter", name, filters[0])
		}
	}
}

func TestSRv6End(t *testing.T) {
	requirePrivileges(t)
	objs, err := LoadSRv6End(PinPaths{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer objs.Close()

	src, final := net.ParseIP("fd00::1"), net.ParseIP("fd00::100")
	sid1, sid2 := net.ParseIP("fd00::a"), net.ParseIP("fd00::b")
	srv6 := func(sl uint8) []byte {
		// Segment List[0] is the final destination; the active segment is Segment List[sl].
		segs := []net.IP{final, sid1, sid2}
		h := srh(unix.IPPROTO_UDP, sl, segs...)
		return packet(ethHdr(unix.ETH_P_IPV6), ipv6Hdr(unix.IPPROTO_ROUTING, src, segs[sl], len(h)+8), h, udpHdr(1000, 2000))
	}

	t.Run("End decrements SL and sets DA to the next segment", func(t *testing.T) {
		ret, out := run(t, objs.Srv6End, srv6(2))
		if ret != tcActOK {
			t.Fatalf("ret = %d, want TC_ACT_OK", ret)
		}
		ip6 := out[14:]
		if sl := ip6[40+3]; sl != 1 {
			t.Errorf("segments_left = %d, want 1", sl)
		}
		if da := net.IP(ip6[24:40]); !da.Equal(sid1) {
			t.Errorf("DA = %s, want %s", da, sid1)
		}
	})
	t.Run("End on the last segment reaches the final destination", func(t *testing.T) {
		_, out := run(t, objs.Srv6End, srv6(1))
		if da := net.IP(out[14+24 : 14+40]); !da.Equal(final) {
			t.Errorf("DA = %s, want %s", da, final)
		}
	})
	t.Run("SL=0 is dropped", func(t *testing.T) {
		if ret, _ := run(t, objs.Srv6End, srv6(0)); ret != tcActShot {
			t.Fatalf("ret = %d, want TC_ACT_SHOT", ret)
		}
	})
	t.Run("hdrlen too short for the segment list is dropped", func(t *testing.T) {
		pkt := srv6(2)
		pkt[14+40+1] = 2 // covers one segment, but last_entry says three
		if ret, _ := run(t, objs.Srv6End, pkt); ret != tcActShot {
			t.Fatalf("ret = %d, want TC_ACT_SHOT", ret)
		}
	})

	passthrough := map[string][]byte{
		"plain IPv6": packet(ethHdr(unix.ETH_P_IPV6), ipv6Hdr(unix.IPPROTO_UDP, src, final, 8), udpHdr(1000, 2000),
			make([]byte, 40)),
		"IPv4": packet(ethHdr(unix.ETH_P_IP), ipv4Hdr(unix.IPPROTO_UDP, net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2), 8),
			udpHdr(1000, 2000), make([]byte, 64)),
	}
	for name, pkt := range passthrough {
		t.Run(name+" passes untouched", func(t *testing.T) {
			ret, out := run(t, objs.Srv6End, pkt)
			if ret != tcActOK {
				t.Fatalf("ret = %d, want TC_ACT_OK", ret)
			}
			if !bytes.Equal(out, pkt) {
				t.Fatal("packet was modified")
			}
		})
	}
}

func TestFillBuckets(t *testing.T) {
	requirePrivileges(t)
	objs, err := LoadSRv6Dispatch(PinPaths{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer objs.Close()

	check := func(t *testing.T, want func(b uint32) Srv6DispatchInstance) {
		t.Helper()
		for b := uint32(0); b < objs.Buckets.MaxEntries(); b++ {
			var got Srv6DispatchInstance
			if err := objs.Buckets.Lookup(b, &got); err != nil {
				t.Fatal(err)
			}
			if w := want(b); got.Ifindex != w.Ifindex || got.Mac != w.Mac {
				t.Fatalf("bucket %d = %+v, want %+v", b, got, w)
			}
		}
	}

	instances := []Srv6DispatchInstance{
		{Ifindex: 10, Mac: [6]byte{0x02, 0xbb, 0, 0, 0, 1}},
		{Ifindex: 11, Mac: [6]byte{0x02, 0xbb, 0, 0, 0, 2}},
		{Ifindex: 12, Mac: [6]byte{0x02, 0xbb, 0, 0, 0, 3}},
	}
	if err := FillBuckets(objs.Buckets, instances); err != nil {
		t.Fatal(err)
	}
	check(t, func(b uint32) Srv6DispatchInstance { return instances[b%3] })

	if err := FillBuckets(objs.Buckets, nil); err != nil {
		t.Fatal(err)
	}
	check(t, func(uint32) Srv6DispatchInstance { return Srv6DispatchInstance{} })
}

func TestSRv6Dispatch(t *testing.T) {
	requirePrivileges(t)
	objs, err := LoadSRv6Dispatch(PinPaths{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer objs.Close()

	// BPF_PROG_TEST_RUN injects packets on the loopback device, so that is the ingress ifindex
	// the program compares against cfg.dispatch_ifindex.
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	dispatchPort := uint32(lo.Index)
	const otherPort = 5 // any ifindex but lo's makes lo a return port
	vmac := [6]byte{0x02, 0xaa, 0, 0, 0, 1}
	setCfg := func(t *testing.T, dispatchIfindex uint32) {
		t.Helper()
		if err := objs.Cfg.Put(uint32(0), Srv6DispatchConfig{DispatchIfindex: dispatchIfindex, Vmac: vmac}); err != nil {
			t.Fatal(err)
		}
	}
	setBuckets := func(t *testing.T, instance func(b uint32) Srv6DispatchInstance) {
		t.Helper()
		for b := uint32(0); b < 256; b++ {
			if err := objs.Buckets.Put(b, instance(b)); err != nil {
				t.Fatal(err)
			}
		}
	}
	clearBuckets := func(t *testing.T) {
		t.Helper()
		setBuckets(t, func(uint32) Srv6DispatchInstance { return Srv6DispatchInstance{} })
	}
	// Fill every bucket with a distinct instance, so the instance MAC on the output identifies
	// the bucket the flow hashed to.
	fillBuckets := func(t *testing.T) {
		t.Helper()
		setBuckets(t, func(b uint32) Srv6DispatchInstance {
			return Srv6DispatchInstance{Ifindex: 100 + b, Mac: [6]byte{0x02, 0xbb, 0, 0, 0, byte(b)}}
		})
	}
	a, b := net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2)
	udp4 := func(dst [6]byte, src, dstIP net.IP, sport, dport uint16) []byte {
		eth := ethHdr(unix.ETH_P_IP)
		copy(eth[0:6], dst[:])
		return packet(eth, ipv4Hdr(unix.IPPROTO_UDP, src, dstIP, 8), udpHdr(sport, dport), make([]byte, 32))
	}
	dispatch := func(t *testing.T, pkt []byte) [6]byte {
		t.Helper()
		ret, out := run(t, objs.Srv6Dispatch, pkt)
		if ret != tcActRedirect {
			t.Fatalf("ret = %d, want TC_ACT_REDIRECT", ret)
		}
		return [6]byte(out[0:6])
	}

	t.Run("unconfigured target is dropped", func(t *testing.T) {
		setCfg(t, 0)
		fillBuckets(t)
		if ret, _ := run(t, objs.Srv6Dispatch, udp4(vmac, a, b, 1, 2)); ret != tcActShot {
			t.Fatalf("ret = %d, want TC_ACT_SHOT", ret)
		}
	})
	t.Run("dispatch with an empty bucket is dropped", func(t *testing.T) {
		setCfg(t, dispatchPort)
		clearBuckets(t)
		if ret, _ := run(t, objs.Srv6Dispatch, udp4(vmac, a, b, 1, 2)); ret != tcActShot {
			t.Fatalf("ret = %d, want TC_ACT_SHOT", ret)
		}
	})
	t.Run("dispatch drops frames not addressed to the vMAC", func(t *testing.T) {
		setCfg(t, dispatchPort)
		fillBuckets(t)
		other := [6]byte{0x02, 0xcc, 0, 0, 0, 1}
		if ret, _ := run(t, objs.Srv6Dispatch, udp4(other, a, b, 1, 2)); ret != tcActShot {
			t.Fatalf("ret = %d, want TC_ACT_SHOT", ret)
		}
	})
	t.Run("dispatch rewrites the dst MAC to an instance", func(t *testing.T) {
		setCfg(t, dispatchPort)
		fillBuckets(t)
		mac := dispatch(t, udp4(vmac, a, b, 1000, 2000))
		if mac[0] != 0x02 || mac[1] != 0xbb {
			t.Fatalf("dst MAC = %x, want an instance MAC", mac)
		}
	})
	t.Run("dispatch is symmetric", func(t *testing.T) {
		setCfg(t, dispatchPort)
		fillBuckets(t)
		fwd := dispatch(t, udp4(vmac, a, b, 1000, 2000))
		rev := dispatch(t, udp4(vmac, b, a, 2000, 1000))
		if fwd != rev {
			t.Fatalf("A->B went to %x, B->A went to %x", fwd, rev)
		}
	})
	t.Run("dispatch hashes the inner flow of H.Encaps packets", func(t *testing.T) {
		setCfg(t, dispatchPort)
		fillBuckets(t)
		inner := packet(ipv4Hdr(unix.IPPROTO_UDP, a, b, 8), udpHdr(1000, 2000))
		encap := func(outerSrc string) []byte {
			sid := net.ParseIP("fd00::a")
			h := srh(unix.IPPROTO_IPIP, 0, sid)
			eth := ethHdr(unix.ETH_P_IPV6)
			copy(eth[0:6], vmac[:])
			return packet(eth, ipv6Hdr(unix.IPPROTO_ROUTING, net.ParseIP(outerSrc), sid, len(h)+len(inner)), h, inner)
		}
		plain := dispatch(t, udp4(vmac, a, b, 1000, 2000))
		for _, outerSrc := range []string{"fd00::1", "fd00::2"} {
			if mac := dispatch(t, encap(outerSrc)); mac != plain {
				t.Errorf("encapsulated from %s went to %x, inner flow alone went to %x", outerSrc, mac, plain)
			}
		}
	})
	t.Run("return rewrites the src MAC to the vMAC", func(t *testing.T) {
		setCfg(t, otherPort)
		ret, out := run(t, objs.Srv6Dispatch, udp4([6]byte{0x02, 0xdd, 0, 0, 0, 1}, a, b, 1, 2))
		if ret != tcActRedirect {
			t.Fatalf("ret = %d, want TC_ACT_REDIRECT", ret)
		}
		if src := [6]byte(out[6:12]); src != vmac {
			t.Fatalf("src MAC = %x, want vMAC %x", src, vmac)
		}
	})
	t.Run("return drops multicast and broadcast", func(t *testing.T) {
		setCfg(t, otherPort)
		bcast := [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		if ret, _ := run(t, objs.Srv6Dispatch, udp4(bcast, a, b, 1, 2)); ret != tcActShot {
			t.Fatalf("ret = %d, want TC_ACT_SHOT", ret)
		}
	})
}

// run executes prog on pkt with BPF_PROG_TEST_RUN and returns the verdict and the packet
// as the program left it.
func run(t *testing.T, prog *ebpf.Program, pkt []byte) (uint32, []byte) {
	t.Helper()
	opts := &ebpf.RunOptions{Data: pkt, DataOut: make([]byte, len(pkt)+256)}
	ret, err := prog.Run(opts)
	if err != nil {
		t.Fatalf("test run: %v", err)
	}
	return ret, opts.DataOut
}

func packet(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func ethHdr(proto uint16) []byte {
	h := make([]byte, 14)
	copy(h[0:6], []byte{0x02, 0, 0, 0, 0, 2})
	copy(h[6:12], []byte{0x02, 0, 0, 0, 0, 1})
	binary.BigEndian.PutUint16(h[12:], proto)
	return h
}

func ipv6Hdr(nextHdr uint8, src, dst net.IP, payloadLen int) []byte {
	h := make([]byte, 40)
	h[0] = 0x60
	binary.BigEndian.PutUint16(h[4:], uint16(payloadLen))
	h[6] = nextHdr
	h[7] = 64
	copy(h[8:24], src.To16())
	copy(h[24:40], dst.To16())
	return h
}

// srh builds a Segment Routing Header with the given Segment List, where segs[0] is the
// final destination.
func srh(nextHdr uint8, segmentsLeft uint8, segs ...net.IP) []byte {
	h := make([]byte, 8, 8+16*len(segs))
	h[0] = nextHdr
	h[1] = uint8(2 * len(segs))
	h[2] = 4 // routing type: SRH
	h[3] = segmentsLeft
	h[4] = uint8(len(segs) - 1)
	for _, s := range segs {
		h = append(h, s.To16()...)
	}
	return h
}

func ipv4Hdr(proto uint8, src, dst net.IP, payloadLen int) []byte {
	h := make([]byte, 20)
	h[0] = 0x45
	binary.BigEndian.PutUint16(h[2:], uint16(20+payloadLen))
	h[8] = 64
	h[9] = proto
	copy(h[12:16], src.To4())
	copy(h[16:20], dst.To4())
	return h
}

func udpHdr(sport, dport uint16) []byte {
	h := make([]byte, 8)
	binary.BigEndian.PutUint16(h[0:], sport)
	binary.BigEndian.PutUint16(h[2:], dport)
	binary.BigEndian.PutUint16(h[4:], 8)
	return h
}
