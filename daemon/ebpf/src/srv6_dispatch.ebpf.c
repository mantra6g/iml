// SPDX-License-Identifier: GPL-2.0
/*
 * nf_dispatch.bpf.c - per-function flow dispatcher for multi-instance NFs.
 *
 * Per function F (namespace F with full-duplex nf0..nfN-1), namespace A has:
 *
 *   shared bridge ── veth-sh-f ══ veth-f ──┬──► f-nf0 ══ F:nf0
 *     [srv6_end on veth-sh-f ingress]      ├──► f-nf1 ══ F:nf1
 *                                          └──► ...
 *
 * veth-f's MAC is the function's "virtual MAC" (vMAC): the only MAC the
 * bridge and A's neighbor table ever see for F.
 *
 * One program (nf_port) on tc ingress of veth-f and every f-nfX. Role comes
 * from `port_cfg`, keyed by ingress ifindex:
 *
 *   DISPATCH (veth-f): accept frames for vMAC only, hash the flow
 *                      (inner 5-tuple for SRv6 H.Encaps), pick a bucket,
 *                      dst MAC := instance MAC, redirect to f-nfX.
 *   RETURN   (f-nfX):  unicast only, src MAC := vMAC, redirect to veth-f
 *                      (-> veth-sh-f -> srv6_end -> bridge -> A's stack).
 *
 * The hash is symmetric (A->B and B->A of a flow pick the same instance) and
 * goes through a 256-entry bucket table per function, so adding, removing or
 * draining an instance only moves the buckets you rewrite.
 */
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#define NBUCKETS	256		/* per function, power of two */
#define MAX_FUNCS	64
#define MAX_EXT_HDRS	6

enum { ROLE_DISPATCH = 1, ROLE_RETURN = 2 };

struct port_cfg {
	__u32 role;
	__u32 arg;			/* DISPATCH: function id, RETURN: ifindex of veth-f */
	__u8  vmac[ETH_ALEN];		/* function's virtual MAC */
	__u8  pad[2];
};

struct instance {
	__u32 ifindex;			/* A-side peer of nfX (0 = bucket unused) */
	__u8  mac[ETH_ALEN];		/* MAC of nfX */
	__u8  pad[2];
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1024);
	__type(key, __u32);
	__type(value, struct port_cfg);
} port_cfg SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, MAX_FUNCS * NBUCKETS);
	__type(key, __u32);		/* fid * NBUCKETS + bucket */
	__type(value, struct instance);
} buckets SEC(".maps");

struct tuple {
	__u32 src[4], dst[4];		/* IPv4 uses [0] */
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
				    const struct port_cfg *c)
{
	struct tuple t = {};
	struct instance *in;
	__u8 dmac[ETH_ALEN];
	__u16 proto;
	__u32 idx;

	if (bpf_skb_load_bytes(skb, 0, dmac, ETH_ALEN) ||
	    bpf_skb_load_bytes(skb, 12, &proto, sizeof(proto)))
		return TC_ACT_SHOT;

#pragma unroll
	for (int i = 0; i < ETH_ALEN; i++)
		if (dmac[i] != c->vmac[i])
			return TC_ACT_SHOT;	/* floods, other functions' frames */

	if (proto == bpf_htons(ETH_P_IPV6))
		parse_outer_ipv6(skb, &t);
	else if (proto == bpf_htons(ETH_P_IP))
		parse_ipv4(skb, sizeof(struct ethhdr), &t);
	/* anything else hashes to a constant bucket */

	if (c->arg >= MAX_FUNCS)
		return TC_ACT_SHOT;
	idx = c->arg * NBUCKETS + (flow_hash(&t) & (NBUCKETS - 1));
	in = bpf_map_lookup_elem(&buckets, &idx);
	if (!in || !in->ifindex)
		return TC_ACT_SHOT;

	if (bpf_skb_store_bytes(skb, 0, in->mac, ETH_ALEN, 0))
		return TC_ACT_SHOT;
	return bpf_redirect(in->ifindex, 0);
}

static __always_inline int ret(struct __sk_buff *skb, const struct port_cfg *c)
{
	__u8 d0;

	if (bpf_skb_load_bytes(skb, 0, &d0, 1))
		return TC_ACT_SHOT;
	if (d0 & 1)				/* NF's own multicast/broadcast stays home */
		return TC_ACT_SHOT;
	if (bpf_skb_store_bytes(skb, ETH_ALEN, c->vmac, ETH_ALEN, 0))
		return TC_ACT_SHOT;
	return bpf_redirect(c->arg, 0);
}

SEC("tc")
int nf_port(struct __sk_buff *skb)
{
	__u32 key = skb->ifindex;
	struct port_cfg* c = bpf_map_lookup_elem(&port_cfg, &key);

	if (!c)
		return TC_ACT_SHOT;
	if (c->role == ROLE_DISPATCH)
		return dispatch(skb, c);
	if (c->role == ROLE_RETURN)
		return ret(skb, c);
	return TC_ACT_SHOT;
}

char LICENSE[] SEC("license") = "GPL";