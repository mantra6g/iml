// SPDX-License-Identifier: GPL-2.0
/*
 * srv6_dispatch.ebpf.c - per-target flow dispatcher (one loaded instance per target).
 *
 * Load this object once PER TARGET and pin it under that target's own
 * directory, e.g.
 *
 *   /sys/fs/bpf/loom/targets/<target_name>/programs/srv6_dispatch     program
 *   /sys/fs/bpf/loom/targets/<target_name>/maps/cfg                   this target's config
 *   /sys/fs/bpf/loom/targets/<target_name>/maps/buckets               this target's bucket table
 *
 * Per target T, the host's namespace has:
 *
 *   routing bridge ── bridge-veth ══ target-veth ──┬──► tgt-if0 ══ T:nf0
 *                                                  ├──► tgt-if1 ══ T:nf1
 *                                                  └──► ...
 *
 * Attach srv6_dispatch to tc ingress of target-veth and of every tgt-nfX. Role is implicit:
 *
 *   ingress on target-veth (config.dispatch_ifindex)  -> DISPATCH
 *       accept frames for the vMAC only, hash the flow (inner 5-tuple for
 *       SRv6 H.Encaps, symmetric), bucket -> instance, dst MAC := instance
 *       MAC, redirect to tgt-nfX.
 *   ingress on any other port it is attached to  -> RETURN
 *       unicast only, src MAC := vMAC, redirect to target-veth
 *       (-> bridge-veth -> srv6_end -> bridge -> host's stack).
 *
 * Because every map belongs to exactly one function there is no function ID,
 * no global registry, and deleting a function is "detach + rm -r its dir".
 */
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#define NBUCKETS     256        /* power of two */
#define MAX_EXT_HDRS 6

struct config {
	__u32 dispatch_ifindex;		/* target-veth (0 = not configured: drop all) */
	__u8  vmac[ETH_ALEN];		/* function's virtual MAC (target-veth's MAC) */
	__u8  pad[2];
};

struct instance {
	__u32 ifindex;              /* host-side peer of the instance (0 = unused) */
	__u8  mac[ETH_ALEN];        /* MAC of the instance's interface */
	__u8  pad[2];
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct config);
} cfg SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, NBUCKETS);
	__type(key, __u32);
	__type(value, struct instance);
} buckets SEC(".maps");

struct tuple {
	__u32 src[4], dst[4];       /* IPv4 uses [0] */
	__u16 sport, dport;
	__u8  proto;
};

/* ------------------------------------------------------------------ hash */

static __always_inline __u32 fmix(__u32 h)
{
	h ^= h >> 16; h *= 0x85ebca6b;
	h ^= h >> 13; h *= 0xc2b2ae35;
	h ^= h >> 16;
	return h;
}

static __always_inline __u32 mix(__u32 h, __u32 v)
{
	return fmix(h ^ (v * 0x9e3779b1));
}

static __always_inline __u32 fold_addr(const __u32 *a)
{
	return mix(mix(mix(mix(0, a[0]), a[1]), a[2]), a[3]);
}

/* Symmetric: endpoints (addr, port) are put in canonical order first. */
static __always_inline __u32 flow_hash(const struct tuple *t)
{
	__u32 ha = fold_addr(t->src), hb = fold_addr(t->dst);
	__u32 pa = t->sport, pb = t->dport, tmp;

	if (ha > hb || (ha == hb && pa > pb)) {
		tmp = ha; ha = hb; hb = tmp;
		tmp = pa; pa = pb; pb = tmp;
	}
	return mix(mix(mix(mix(0x5eed, ha), hb), (pa << 16) | pb), t->proto);
}

/* --------------------------------------------------------------- parsing */

static __always_inline void get_ports(struct __sk_buff *skb, __u32 off,
				      struct tuple *t)
{
	__u16 p[2];

	if (t->proto != IPPROTO_TCP && t->proto != IPPROTO_UDP &&
	    t->proto != IPPROTO_SCTP)
		return;
	if (!bpf_skb_load_bytes(skb, off, p, sizeof(p))) {
		t->sport = p[0];
		t->dport = p[1];
	}
}

/*
 * Skip Hop-by-Hop / Routing / Destination Options. Returns the upper-layer
 * protocol and advances *off. If an SRH (routing type 4) is seen, *srh_off
 * gets its offset.
 */
static __always_inline __u8 skip_ipv6_ext(struct __sk_buff *skb, __u32 *off,
					  __u8 nh, __u32 *srh_off)
{
	struct { __u8 nh, len, type, sl; } h;

#pragma unroll
	for (int i = 0; i < MAX_EXT_HDRS; i++) {
		if (nh != IPPROTO_HOPOPTS && nh != IPPROTO_ROUTING &&
		    nh != IPPROTO_DSTOPTS)
			return nh;
		if (bpf_skb_load_bytes(skb, *off, &h, sizeof(h)))
			return IPPROTO_NONE;
		if (nh == IPPROTO_ROUTING && h.type == 4)
			*srh_off = *off;
		*off += ((__u32)h.len + 1) * 8;
		nh = h.nh;
	}
	return IPPROTO_NONE;
}

static __always_inline void parse_ipv4(struct __sk_buff *skb, __u32 off,
				       struct tuple *t)
{
	struct iphdr ip;

	if (bpf_skb_load_bytes(skb, off, &ip, sizeof(ip)))
		return;
	t->src[0] = ip.saddr;
	t->dst[0] = ip.daddr;
	t->proto  = ip.protocol;
	if (ip.frag_off & bpf_htons(0x3fff))	/* any fragment: no ports */
		return;
	get_ports(skb, off + ip.ihl * 4, t);
}

static __always_inline void parse_ipv6(struct __sk_buff *skb, __u32 off,
				       struct tuple *t)
{
	struct ipv6hdr ip6;
	__u32 srh_off = 0;

	if (bpf_skb_load_bytes(skb, off, &ip6, sizeof(ip6)))
		return;
	__builtin_memcpy(t->src, &ip6.saddr, 16);
	__builtin_memcpy(t->dst, &ip6.daddr, 16);
	off += sizeof(ip6);
	t->proto = skip_ipv6_ext(skb, &off, ip6.nexthdr, &srh_off);
	if (t->proto != IPPROTO_FRAGMENT)	/* fragments: addresses only */
		get_ports(skb, off, t);
}

/*
 * Outer IPv6:
 *   SRv6 H.Encaps (SRH -> IPv4/IPv6): hash the inner packet.
 *   Inline SRH or plain IPv6:         hash outer src + FINAL destination
 *                                     (Segment List[0]), since the DA changes
 *                                     at every End.
 */
static __always_inline void parse_outer_ipv6(struct __sk_buff *skb,
					     struct tuple *t)
{
	struct ipv6hdr ip6;
	__u32 off = sizeof(struct ethhdr), srh_off = 0;
	__u8 nh;

	if (bpf_skb_load_bytes(skb, off, &ip6, sizeof(ip6)))
		return;
	off += sizeof(ip6);
	nh = skip_ipv6_ext(skb, &off, ip6.nexthdr, &srh_off);

	if (nh == IPPROTO_IPIP) {
		parse_ipv4(skb, off, t);
		return;
	}
	if (nh == IPPROTO_IPV6) {
		parse_ipv6(skb, off, t);
		return;
	}

	__builtin_memcpy(t->src, &ip6.saddr, 16);
	__builtin_memcpy(t->dst, &ip6.daddr, 16);
	if (srh_off)	/* Segment List[0] = final destination */
		bpf_skb_load_bytes(skb, srh_off + 8, t->dst, 16);
	t->proto = nh;
	if (nh != IPPROTO_FRAGMENT)
		get_ports(skb, off, t);
}

/* ------------------------------------------------------------- datapath */

static __always_inline int dispatch(struct __sk_buff *skb,
				    const struct config *c)
{
	struct tuple t = {};
	struct instance *in;
	__u8 dmac[ETH_ALEN];
	__u16 proto;
	__u32 bucket;

	if (bpf_skb_load_bytes(skb, 0, dmac, ETH_ALEN) ||
	    bpf_skb_load_bytes(skb, 12, &proto, sizeof(proto)))
		return TC_ACT_SHOT;

#pragma unroll
	for (int i = 0; i < ETH_ALEN; i++)
		if (dmac[i] != c->vmac[i])
			return TC_ACT_SHOT;	/* floods, frames not for us */

	if (proto == bpf_htons(ETH_P_IPV6))
		parse_outer_ipv6(skb, &t);
	else if (proto == bpf_htons(ETH_P_IP))
		parse_ipv4(skb, sizeof(struct ethhdr), &t);
	/* anything else hashes to a constant bucket */

	bucket = flow_hash(&t) & (NBUCKETS - 1);
	in = bpf_map_lookup_elem(&buckets, &bucket);
	if (!in || !in->ifindex)
		return TC_ACT_SHOT;

	if (bpf_skb_store_bytes(skb, 0, in->mac, ETH_ALEN, 0))
		return TC_ACT_SHOT;
	return bpf_redirect(in->ifindex, 0);
}

static __always_inline int ret(struct __sk_buff *skb, const struct config *c)
{
	__u8 d0;

	if (bpf_skb_load_bytes(skb, 0, &d0, 1))
		return TC_ACT_SHOT;
	if (d0 & 1)			/* instance's own multicast/broadcast stays home */
		return TC_ACT_SHOT;
	if (bpf_skb_store_bytes(skb, ETH_ALEN, c->vmac, ETH_ALEN, 0))
		return TC_ACT_SHOT;
	return bpf_redirect(c->dispatch_ifindex, 0);
}

SEC("tc")
int srv6_dispatch(struct __sk_buff *skb)
{
	__u32 zero = 0;
	struct config* c = bpf_map_lookup_elem(&cfg, &zero);

	if (!c || !c->dispatch_ifindex)
		return TC_ACT_SHOT;
	if (skb->ifindex == c->dispatch_ifindex)
		return dispatch(skb, c);
	return ret(skb, c);
}

char LICENSE[] SEC("license") = "GPL";