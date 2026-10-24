//go:build linux

package ebpf

import (
	"fmt"

	"github.com/cilium/ebpf"
)

// FillBuckets spreads every bucket of a srv6_dispatch "buckets" map over instances
// round-robin, so bucket b is served by instances[b % len(instances)]. With no instances
// every bucket is cleared, and the dispatcher drops all traffic.
func FillBuckets(buckets *ebpf.Map, instances []Srv6DispatchInstance) error {
	n := uint32(len(instances))
	for b := uint32(0); b < buckets.MaxEntries(); b++ {
		var in Srv6DispatchInstance
		if n > 0 {
			in = instances[b%n]
		}
		if err := buckets.Put(b, in); err != nil {
			return fmt.Errorf("failed to update bucket %d: %w", b, err)
		}
	}
	return nil
}
