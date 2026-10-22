// SPDX-License-Identifier: GPL-2.0
/*
 * srv6_end.bpf.c - SRv6 End behavior.
 *
 * Attach to TC ingress of the host-side pod veths.
 *
 * This program applies the End behavior (RFC 8986, 4.1):
 *
 *     SL = SL - 1
 *     IPv6 DA = Segment List[SL]
 *
 * and returns TC_ACT_OK, which passes the frame up to the host's IPv6 stack, 
 * which routes it to the next SID (route lookup, hop-limit decrement, 
 * NDP and ICMP errors are all handled by the kernel).
 *
 * Non-SRv6 traffic, and SRv6 packets with SL == 0, are passed through untouched.
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

SEC("tc/ingress")
int srv6_end(struct __sk_buff *skb)
{
	void *data, *data_end;
	struct ethhdr *eth;
	struct ipv6hdr *ip6;
	struct ipv6_sr_hdr *srh;
	struct in6_addr *next;
	__u32 sl, need;

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
		return TC_ACT_OK;			/* not SRv6: bridge normally */

	if (srh->segments_left == 0)
		return TC_ACT_OK;			/* final destination handles it */

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

	/* bpf_skb_pull_data() invalidates packet pointers: reload. */
	data     = (void *)(long)skb->data;
	data_end = (void *)(long)skb->data_end;

	ip6 = data + sizeof(struct ethhdr);
	srh = (void *)(ip6 + 1);
	if ((void *)(srh + 1) > data_end)
		return TC_ACT_SHOT;

	next = &srh->segments[sl];
	if ((void *)(next + 1) > data_end)
		return TC_ACT_SHOT;

	srh->segments_left = sl;
	ip6->daddr = *next;

	/*
	 * No checksum fixup: IPv6 has no header checksum, and the L4
	 * pseudo-header uses the final destination (Segment List[0]),
	 * which End never changes.
	 */
	return TC_ACT_OK;
}

char LICENSE[] SEC("license") = "GPL";