// SPDX-License-Identifier: GPL-2.0
/*
 * srv6_end.ebpf.c - SRv6 End behavior.
 *
 * Attaches to the TC ingress of the bridge-side of the routing bridge.
 *
 * End behavior (RFC 8986, 4.1):
 *     SL = SL - 1
 *     IPv6 DA = Segment List[SL]
 *
 * Returns TC_ACT_OK so the frame continues up the stack.
 * Non-SRv6 traffic is passed through untouched.
 * SRv6 traffic with SL=0 is dropped.
 *
 * Assumptions: untagged Ethernet, SRH immediately follows the IPv6 header.
 */
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/in6.h>
#include <linux/ipv6.h>
#include <linux/seg6.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#define MAX_SEGS 16

#define FIXED_HDRS (sizeof(struct ethhdr) + sizeof(struct ipv6hdr) + \
		    sizeof(struct ipv6_sr_hdr))

struct mac {
	__u8 b[ETH_ALEN];
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct mac);
} br_mac SEC(".maps");

SEC("tc")
int srv6_end(struct __sk_buff *skb)
{
	void *data, *data_end;
	struct ethhdr *eth;
	struct ipv6hdr *ip6;
	struct ipv6_sr_hdr *srh;
	struct in6_addr *next;
	struct mac *m;
	__u32 sl, need, zero = 0;

	/* Make Ethernet + IPv6 + fixed SRH part linear and writable. */
	if (skb->len < FIXED_HDRS || bpf_skb_pull_data(skb, FIXED_HDRS))
		return TC_ACT_OK;

	data     = (void *)(long)skb->data;
	data_end = (void *)(long)skb->data_end;

	eth = data;
	if ((void *)(eth + 1) > data_end)
		return TC_ACT_OK;
	if (eth->h_proto != bpf_htons(ETH_P_IPV6))
		return TC_ACT_OK;

	ip6 = (void *)(eth + 1);
	srh = (void *)(ip6 + 1);
	if ((void *)(srh + 1) > data_end)
		return TC_ACT_OK;

	if (ip6->nexthdr != IPPROTO_ROUTING || srh->type != IPV6_SRCRT_TYPE_4)
		return TC_ACT_OK;		/* not SRv6: bridge normally */

	if (srh->segments_left == 0)
		return TC_ACT_SHOT;		/* SL=0: drop */

	/* ---- End behavior ---- */
	sl = srh->segments_left - 1;
	if (sl > srh->first_segment || sl >= MAX_SEGS)
		return TC_ACT_SHOT;

	/* hdrlen (8-octet units, excl. first 8) must cover the segment list */
	if ((__u32)srh->hdrlen * 8 <
	    ((__u32)srh->first_segment + 1) * sizeof(struct in6_addr))
		return TC_ACT_SHOT;

	/* Pull up to and including Segment List[sl]. */
	need = FIXED_HDRS + (sl + 1) * sizeof(struct in6_addr);
	if (skb->len < need || bpf_skb_pull_data(skb, need))
		return TC_ACT_SHOT;

	/* bpf_skb_pull_data() invalidates packet pointers: reload and re-check. */
	data     = (void *)(long)skb->data;
	data_end = (void *)(long)skb->data_end;

	eth = data;
	if ((void *)(eth + 1) > data_end)
		return TC_ACT_SHOT;

	ip6 = (void *)(eth + 1);
	srh = (void *)(ip6 + 1);
	if ((void *)(srh + 1) > data_end)
		return TC_ACT_SHOT;

	next = &srh->segments[sl];
	if ((void *)(next + 1) > data_end)
		return TC_ACT_SHOT;

	srh->segments_left = sl;
	ip6->daddr = *next;

	m = bpf_map_lookup_elem(&br_mac, &zero);
	if (m && (m->b[0] | m->b[1] | m->b[2] | m->b[3] | m->b[4] | m->b[5])) {
		__builtin_memcpy(eth->h_dest, m->b, ETH_ALEN);
	}

	/*
	 * No checksum fixup: IPv6 has no header checksum, and the L4
	 * pseudo-header uses the final destination (Segment List[0]),
	 * which End never changes.
	 */
	return TC_ACT_OK;
}

char LICENSE[] SEC("license") = "GPL";