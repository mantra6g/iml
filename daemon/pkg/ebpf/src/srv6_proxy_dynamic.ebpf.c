#include "proxy.h"

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32); // the lane (id'ing the SRv6 context) of the cached header
    __type(value, struct cached_outer); // the cached header structure (lock + len + IPv6 + SRH)
} srh_table SEC(".maps");

// not modified by xdp programs
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32); // lane-dependent key associated with the fromVNF
    __type(value, struct mac_entry); // the mac addresses fromVNF interface belonging to this lane 
} mac_addr_table SEC(".maps");

// not modified by xdp programs
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32);
    __type(value, __u32); // the associated toVNF interface to put out decapsulated traffic
} link_index_table SEC(".maps");

// init for each CPU with scratch space to update hdr in encap
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct scratch_outer);
} encap_scratch_table SEC(".maps");


/*
DECAP FUNCTIONS
*/

static __always_inline
int parse_packet(struct parsed_srh *p, void *start, void *end)
{
    p->eth = start;
    if((void *)(p->eth + 1) > end)
        return -1;

    p->ip6 = (void *)(p->eth + 1);
    if((void *)(p->ip6 + 1) > end)
        return -1;

    p->srh = (void *)(p->ip6 + 1);
    // check for fixed len routing header first [ NextHeader | HdrExtLen | RoutingType | SegmentsLeft ]
    if((void *)((char *)p->srh + 8) > end)  
        return -1;

    __u8 hdr_ext_len = p->srh->hdrlen;
    __u8 last_index = p->srh->first_segment;

    p->srh_len = RH_SIZE + hdr_ext_len * 8;
    if(p->srh_len > MAX_SRH_LEN)
        return -1;

    if((void *)((char *)p->srh + p->srh_len) > end)
        return -1;
    
    p->outer_len = sizeof(struct ipv6hdr) + p->srh_len;
    if((void *)((char *)p->ip6 + p->outer_len) > end)
        return -1;

    if((void *)((char *)p->srh->segments + (p->srh_len - 8)) > end)
        return -1;
    
    p->n_segs = (__u32)last_index + 1; //number of segments in segments list
    
    return 0;
}

static __always_inline
int update_srh(struct parsed_srh *p, void *end)
{   
    if(p->srh_len > MAX_SRH_LEN)
        return -1;
    if((void *)((char *)p->srh + p->srh_len) > end)
        return -1;
    if((void *)((char *)p->srh + 8) > end)
        return -1;
    
    if(p->srh->segments_left < 1)
        return -1;
    p->srh->segments_left--;
    __u8 index = p->srh->segments_left;

    if(index >= (p->srh_len - 8)/sizeof(struct in6_addr)) // ensure index is not bigger than the actual length of the segment list
        return -1;
    if((void *)((struct in6_addr *)p->srh->segments + index + 1) > end)
        return -1;
    
    __builtin_memcpy((void *)&(p->ip6->daddr), (void *)((struct in6_addr *)p->srh->segments + index), sizeof(p->ip6->daddr));

    return 0;
}

static __always_inline
int update_cached_header(struct cached_outer *cached, char *hdr, __u32 *lane, __u32 len, void *end)
{
    if(len > MAX_CACHED_HDR)
        return -1;
    if(sizeof(cached->bytes) < len) // check capacity (not actual length)
        return -1;
    if((void *)(hdr + len) > end)
        return -1;

    bpf_spin_lock(&cached->lock);
    cached->len = len;
    // __builtin_memcpy((void *)cached->bytes, (void *)p->ip6, len);
    for(__u32 i = 0; i < MAX_SRH_LEN + sizeof(struct ipv6hdr); i++){
        if(i >= len)
            break;
        if((void *)(hdr + i + 1) > end){
            bpf_spin_unlock(&cached->lock);
            return -1;
        }
        cached->bytes[i] = hdr[i];
    }
    bpf_spin_unlock(&cached->lock);

    return 0;
}

static __always_inline
int update_eth_hdr(void *start, void *end, __u32 lane)
{
    struct ethhdr *eth = start;
    if((void *)((char *)eth + sizeof(struct ethhdr)) > end)
        return -1;

    __u32 src_key = 2*lane;
    __u32 dst_key = 1 + 2*lane;
    struct mac_entry *src_mac = bpf_map_lookup_elem(&mac_addr_table, &src_key);
    struct mac_entry *dst_mac = bpf_map_lookup_elem(&mac_addr_table, &dst_key);

    if (!src_mac || !dst_mac) 
        return -1;

    __builtin_memcpy((void *)eth->h_source, (void *)src_mac->addr, sizeof(eth->h_source));
    __builtin_memcpy((void *)eth->h_dest, (void *)dst_mac->addr, sizeof(eth->h_dest));

    if((void *)((char *)eth + sizeof(struct ethhdr) + 1) > end)
        return -1;

    __u8 version = *((__u8 *)eth + sizeof(struct ethhdr)) >> 4;
    if(version == 6)
    {
        eth->h_proto = bpf_htons(ETH_P_IPV6);
    }
    else if (version == 4)
    {
        eth->h_proto = bpf_htons(ETH_P_IP);
    } else 
    {
        return -1;
    }

    return 0;
}

static __always_inline
int filter_srv6_traffic(struct parsed_srh *p)
{
    __u8 version = *((__u8 *)(p->ip6)) >> 4;
    if(version != 6){
        return -1;
    }
    if(p->ip6->nexthdr != 43){
        return -1;
    }
    if(p->ip6->hop_limit < 2)
        return -1;
    return 0;
}

/*
ENCAP FUNCTIONS (XDP)
*/

static __always_inline
int copy_cached_to_buffer(struct ethhdr *eth, struct cached_outer *cached, void *end)
{
    if((void *)(eth + 1) > end)
        return -1;

    __u64 *dst = (__u64 *)(eth + 1);

    __u32 len = cached->len;
    if(len > MAX_CACHED_HDR)
        len = MAX_CACHED_HDR;

    __u32 len_64 = len / 8;
    __u32 rest = len % 8;

    if((void *)((char *)dst + len) > end)
        return -1;

    bpf_spin_lock(&cached->lock);

    for(__u32 i = 0; i < MAX_CACHED_HDR / 8; i++){
        if(i >= len_64) break;
        if((void *)(dst + i + 1) > end){
            bpf_spin_unlock(&cached->lock);
            return -1;
        }
        __builtin_memcpy((void *)(dst + i), (void *)((__u64 *)cached->bytes + i), 8);
    }

    // copy the leftover bytes individually
    if(rest){
        __u32 base = len_64 * 8;
        barrier_var(base);                  // keeps clang from proving the check below redundant
        if(base > MAX_CACHED_HDR - 8){      // never true for a valid len; bounds idx for the verifier
            bpf_spin_unlock(&cached->lock);
            return -1;
        }

        for(__u32 i = 0; i < 8; i++){
            if(i >= rest) break;
            __u32 idx = base + i;           // idx <= MAX_CACHED_HDR - 1
            __u8 *d = (__u8 *)dst + idx;
            barrier_var(d);
            if((void *)d >= end){
                bpf_spin_unlock(&cached->lock);
                return -1;
            }
            *d = cached->bytes[idx];
        }
    }

    bpf_spin_unlock(&cached->lock);
    return 0;
}

static __always_inline
int update_ethhdr_encap(struct ethhdr *eth, void *end)
{
    if((void *)(eth + 1) > end)
        return -1;
    
    __u32 src_key = 2;
    __u32 dst_key = 2 + 1;

    struct mac_entry *src = bpf_map_lookup_elem(&mac_addr_table, &src_key);
    struct mac_entry *dst = bpf_map_lookup_elem(&mac_addr_table, &dst_key);

    if(!src || !dst)
        return -1;
    
    // replace mac addresses
    __builtin_memcpy((void *)eth->h_source, (void *)src->addr, sizeof(eth->h_source));
    __builtin_memcpy((void *)eth->h_dest, (void *)dst->addr, sizeof(eth->h_dest));

    // set next proto to IPv6
    eth->h_proto = bpf_htons(ETH_P_IPV6);

    return 0;
}

static __always_inline
int update_nxthdr(struct ethhdr *eth, void *end, __u8 version)
{
    if((void *) eth + 1 > end)
        return -1;

    struct ipv6hdr *ip6 = (struct ipv6hdr *)(eth + 1);
    
    if((void *)(ip6 + 1) > end)
        return -1;
    
    struct ipv6_sr_hdr *srh = (struct ipv6_sr_hdr *)(ip6 + 1);
    if((void *)(srh + 1) > end)
        return -1;
    
    __u32 srh_len = ((__u32)srh->hdrlen + 1) * 8;
    if((void *) ((char *)srh + srh_len) > end)
        return -1;
    void *inner = (void *)srh + srh_len;

    switch (version) {
    case 4: {
        struct iphdr *ip4 = inner;
        if ((void *)(ip4 + 1) > end) return -1;
        ip6->payload_len = bpf_htons(srh_len + bpf_ntohs(ip4->tot_len));
        srh->nexthdr = IPPROTO_IPIP;
        break;
    }
    case 6: {
        struct ipv6hdr *ip6i = inner;
        if ((void *)(ip6i + 1) > end) return -1;
        ip6->payload_len = bpf_htons(srh_len + 40 + bpf_ntohs(ip6i->payload_len));
        srh->nexthdr = IPPROTO_IPV6;
        break;
    }
    default:
        break;
    }
    return 0;
}

/*
DECAP FUNCTIONS (TC)
*/

static __always_inline
int parse_srh_base(struct parsed_srh *p, void *start, void *end)
{
    p->eth = start;
    if((void *)(p->eth + 1) > end)
        return -1;

    p->ip6 = (void *)(p->eth + 1);
    if((void *)(p->ip6 + 1) > end)
        return -1;

    p->srh = (void *)(p->ip6 + 1);
    if((void *)((char *)p->srh + 8) > end)  // only fixed-size base SRH
        return -1;

    __u8 hdr_ext_len = p->srh->hdrlen;
    __u8 last_index = p->srh->first_segment;

    p->srh_len = RH_SIZE + hdr_ext_len * 8;
    if(p->srh_len > MAX_SRH_LEN)
        return -1;

    p->outer_len = sizeof(struct ipv6hdr) + p->srh_len;
    p->n_segs = (__u32)last_index + 1;
    
    return 0;
}

/*
ENCAP FUNCTIONS (TC)
*/

static __always_inline
int copy_cached_to_scratch(__u64 *dst, struct cached_outer *cached)
{

    __u32 len = cached->len;
    if(len > MAX_CACHED_HDR)
        len = MAX_CACHED_HDR;

    __u32 len_64 = len / 8;
    __u32 rest = len % 8;

    bpf_spin_lock(&cached->lock);

    for(__u32 i = 0; i < MAX_CACHED_HDR / 8; i++){
        if(i >= len_64) break;
        __builtin_memcpy((void *)(dst + i), (void *)((__u64 *)cached->bytes + i), 8);
    }

    // copy the leftover bytes individually
    if(rest){
        __u32 base = len_64 * 8;
        barrier_var(base);                  // keeps clang from proving the check below redundant
        if(base > MAX_CACHED_HDR - 8){      // never true for a valid len; bounds idx for the verifier
            bpf_spin_unlock(&cached->lock);
            return -1;
        }

        for(__u32 i = 0; i < 8; i++){
            if(i >= rest) break;
            __u32 idx = base + i;           // idx <= MAX_CACHED_HDR - 1
            __u8 *d = (__u8 *)dst + idx;
            barrier_var(d);
            *d = cached->bytes[idx];
        }
    }

    bpf_spin_unlock(&cached->lock);
    return 0;
}

/*
*
eBPF PROGRAMS
*
*/

SEC("xdp")
int decap(struct xdp_md *ctx)
{   
    void *start = (void *)(long)ctx->data;
    void *end = (void *)(long)ctx->data_end;

    __u32 lane = 0; // assign fixed value for simple dynamic proxy
    struct parsed_srh parsed = {};
    struct parsed_srh *p = &parsed;
    if(parse_packet(p, start, end) < 0)
        return XDP_DROP;

    if(filter_srv6_traffic(p) < 0)
        return XDP_DROP;

    if(update_srh(p, end) < 0)
        return XDP_DROP;

    struct cached_outer *cached;
    cached = bpf_map_lookup_elem(&srh_table, &lane);
    if(!cached)
        return XDP_DROP;

    if(update_cached_header(cached, (char *)p->ip6, &lane, p->outer_len, end) < 0)
        return XDP_DROP;

    struct ethhdr *new_eth = (struct ethhdr *)((char *)p->eth + p->outer_len);
    if((void *)(new_eth + 1) > end)
        return XDP_DROP;
    if((void *)(p->eth + 1) > end)
        return XDP_DROP;
    __builtin_memcpy((void *)new_eth, (void *)p->eth, sizeof(struct ethhdr));
    if(update_eth_hdr(new_eth, end, lane) < 0)
        return XDP_DROP;

    if(bpf_xdp_adjust_head(ctx, p->outer_len) < 0)
        return XDP_DROP;

    // always use 0 for simple dynamic proxy
    __u32 *link_index = bpf_map_lookup_elem(&link_index_table, &lane);
    
    if(!link_index || !*link_index)
        return XDP_DROP;

    return bpf_redirect(*link_index, 0);
}

SEC("xdp")
int encap(struct xdp_md *ctx)
{   
    void *start = (void *)(long)ctx->data;
    void *end = (void *)(long)ctx->data_end;

    struct ethhdr *eth = start;
    if((void *)(eth + 1) > end)
        return XDP_DROP;
    
    char *inner = (char *)(eth + 1);
    if((void *)inner + 1 > end)
        return XDP_DROP;
    
    __u8 version = *((__u8 *)(eth + 1)) >> 4;

    __u32 key = ctx->ingress_ifindex;

    // always use the one cached entry for dynamic proxying
    __u32 lookup_key = 0;
    struct cached_outer *cached = bpf_map_lookup_elem(&srh_table, &lookup_key);
    if(!cached)
        return XDP_DROP;

    __u32 len = cached->len;
    if (len > MAX_CACHED_HDR)
        len = MAX_CACHED_HDR;
    
    if(bpf_xdp_adjust_head(ctx, -(int)len) < 0)
        return XDP_DROP;

    start = (void *)(long)ctx->data;
    end = (void *)(long)ctx->data_end;

    if ((void *)((char *)start + sizeof(struct ethhdr)) > end)
        return XDP_DROP;

    barrier_var(len);
    eth = (struct ethhdr *)((char *)start + len);

    if ((void *)(eth + 1) > end)
        return XDP_DROP;

    __builtin_memcpy(start, eth, sizeof(struct ethhdr));

    eth = (struct ethhdr *)start;

    if(update_ethhdr_encap(eth, end) < 0)
        return XDP_DROP;
    
    if(copy_cached_to_buffer(eth, cached, end) < 0)
        return XDP_DROP;
    
    if(update_nxthdr(eth, end, version) < 0)
        return XDP_DROP;

    key = 2; // per convention the dispatcher veths idx is at index 2
    __u32 *link_index = bpf_map_lookup_elem(&link_index_table, &key);
    if(!link_index || !*link_index)
        return XDP_DROP;

    return bpf_redirect(*link_index, 0);
}

SEC("tc")
int tc_decap(struct __sk_buff *skb)
{
    __u32 pull_len = MAX_ETH_L3_LEN_DECAP;
    if (skb->len < MAX_ETH_L3_LEN_DECAP)
		pull_len = skb->len;
    if(bpf_skb_pull_data(skb, pull_len) < 0)
        return TC_ACT_UNSPEC;

    void *start = (void *)(long)skb->data;
    void *end = (void *)(long)skb->data_end;

    struct parsed_srh parsed = {};
    struct parsed_srh *p = &parsed;
    if(parse_srh_base(p, start, end) < 0)
        return TC_ACT_UNSPEC;

    if(filter_srv6_traffic(p) < 0)
        return TC_ACT_UNSPEC;
    
    if(update_srh(p, end) < 0)
        return TC_ACT_SHOT; // modified the packet -> drop from now on
    __u32 zero_key = 0;
    struct cached_outer *cached;
    cached = bpf_map_lookup_elem(&srh_table, &zero_key);
    if(!cached)
        return TC_ACT_SHOT;
    
    if(update_cached_header(cached, (char *)p->ip6, &zero_key, p->outer_len, end) < 0)
        return TC_ACT_SHOT;
    // copy ethhdr in front of inner packet
    struct ethhdr *new_eth = (struct ethhdr *)((char *)p->eth + p->outer_len); // new location of eth
    if((void *)(new_eth + 1) > end)
        return TC_ACT_SHOT;
    if((void *)(p->eth + 1) > end)
        return TC_ACT_SHOT;
    __builtin_memcpy((void *)new_eth, (void *)p->eth, sizeof(struct ethhdr));

    if((void *)((__u8 *)new_eth + sizeof(struct ethhdr) + 1) > end)
        return TC_ACT_SHOT;
    
    __u8 inner_version = *((__u8 *)new_eth + sizeof(struct ethhdr)) >> 4;
    // TODO: use correct flags to keep skb metadata intact (for specific IML use case it works like this)
    // requires kernel 6.3+, should actually be IPXIPV6
   __u64 flags = PROXY_ADJ_ROOM_FIXED_GSO;
    flags |= (inner_version == 4) ? PROXY_ADJ_ROOM_DECAP_L3_IPV4: PROXY_ADJ_ROOM_DECAP_L3_IPV6;

    __u32 adjust_len = p->outer_len;
    int ret = bpf_skb_adjust_room(skb, -adjust_len, BPF_ADJ_ROOM_MAC, flags);
    if (ret)
        return TC_ACT_SHOT;
    
    start = (void *)(long)skb->data;
    end = (void *)(long)skb->data_end;
    if(update_eth_hdr((struct ethhdr *)start, end, zero_key) < 0)
        return TC_ACT_SHOT;
    __u32 *link_index = bpf_map_lookup_elem(&link_index_table, &zero_key);
    
    if(!link_index || !*link_index)
        return TC_ACT_SHOT;

    return bpf_redirect(*link_index, 0);
}

SEC("tc")
int tc_encap(struct __sk_buff *skb)
{
    __u32 pull_len = MAX_ETH_L3_LEN_ENCAP;
    if (skb->len < MAX_ETH_L3_LEN_ENCAP)
		pull_len = skb->len;
    
    if(bpf_skb_pull_data(skb, pull_len) < 0)
        return TC_ACT_UNSPEC;

    void *start = (void *)(long)skb->data;
	void *end = (void *)(long)skb->data_end;

    struct ethhdr *eth = start;
    if((void *)(eth + 1) > end)
        return TC_ACT_OK;
    
    __u8 *inner = (__u8 *)(eth + 1);
    if((void *)(inner + 1) > end)
        return TC_ACT_OK;

    __u32 version = *(inner) >> 4;

    __be16 proto = eth->h_proto;
    if(!((version == 4 && proto == bpf_htons(ETH_P_IP)) || (version == 6 && proto == bpf_htons(ETH_P_IPV6))))
        return TC_ACT_UNSPEC;
    
    __u32 inner_len = 0;
    if(skb->gso_size != 0){
        inner_len = skb->len - ETH_HLEN;
    } else {
        if(version == 4){
            if((void *)(inner + 4) > end){
                return TC_ACT_UNSPEC;
            }
            inner_len = (__u32)bpf_ntohs(*(__be16 *)(inner + 2));
        } else {
            if((void *)(inner + 6) > end)
                return TC_ACT_UNSPEC;
            inner_len = (__u32)bpf_ntohs(*(__be16 *)(inner + 4)) + 40;
        }
    }

    __u32 map_key = 0;
    struct scratch_outer *scratch = bpf_map_lookup_elem(&encap_scratch_table, &map_key);
    if(!scratch)
        return TC_ACT_UNSPEC;
    
    struct cached_outer *cached = bpf_map_lookup_elem(&srh_table, &map_key);
    if(!cached)
        return TC_ACT_UNSPEC;
    if(copy_cached_to_scratch((__u64 *)scratch->bytes, cached) < 0){
        return TC_ACT_SHOT;
    }
    bpf_spin_lock(&cached->lock);
    scratch->len = cached->len;
    bpf_spin_unlock(&cached->lock);
    __u32 hlen = scratch->len;
    if (hlen < 40 || hlen > MAX_CACHED_HDR) return TC_ACT_UNSPEC;

    // update outer header length
    __u32 payload = hlen - 40 + inner_len;
    __u16 plen = payload > 65535 ? 0 : (__u16)payload;
    *(__be16 *)&scratch->bytes[4] = bpf_htons(plen);

    // update SRH next header
    scratch->bytes[40] = (version == 4) ? IPPROTO_IPIP : IPPROTO_IPV6;
    
    // encap flags are supported for kernel 5.1+
    int ret = bpf_skb_adjust_room(skb, hlen, BPF_ADJ_ROOM_MAC, BPF_F_ADJ_ROOM_ENCAP_L3_IPV6);
    if (ret)
        return TC_ACT_SHOT;
    
    __u64 len = hlen; // ned __u64 to keep clang from changing registers
    barrier_var(len);
    if(len < 40)
        return TC_ACT_SHOT;

    barrier_var(len);
    if(MAX_CACHED_HDR < len)
        return TC_ACT_SHOT;

    if(bpf_skb_store_bytes(skb, ETH_HLEN, scratch->bytes, len, 0) < 0){
        return TC_ACT_SHOT;
    }

    start = (void *)(long)skb->data;
	end = (void *)(long)skb->data_end;
    
    // 6. write eth h_proto, dmac, smac
    if(update_ethhdr_encap(start, end) < 0)
        return TC_ACT_SHOT;

    // 7. return bpf_redirect(oif, 0);
    __u32 key = 2; // per convention the dispatcher veths idx is at index 2
    __u32 *link_index = bpf_map_lookup_elem(&link_index_table, &key);
    if(!link_index || !*link_index)
        return TC_ACT_SHOT;
    return bpf_redirect(*link_index, 0);
}

char LICENSE[] SEC("license") = "GPL";