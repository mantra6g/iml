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
        return XDP_PASS; // pass on, seems to be last segment which we can't handle
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
int cp_ethhdr_in_front_of_inner(struct parsed_srh *p)
{
    __builtin_memcpy((void *)((char *)p->eth + p->outer_len), (void *)p->eth, sizeof(struct ethhdr));
    return 0;
}

static __always_inline
int update_eth_hdr(void *start, void *end, __u32 lane)
{
    struct ethhdr *eth = start;
    if((void *)((char *)eth + sizeof(struct ethhdr)) > end)
        return -1;

    __u32 src_key = lane;
    __u32 dst_key = N + lane;
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

/* static __always_inline
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
} */

/*
ENCAP FUNCTIONS
*/

static __always_inline
int copy_cached_to_buffer(struct ethhdr *eth, struct cached_outer *cached, void *end)
{
    if((void *)(eth + 1) > end)
        return -1;
    __u64 *dst = (__u64 *)(eth + 1);
    if((void *)((char *)dst + cached->len) > end)
        return -1;

    __u32 len = cached->len;
    if(len > MAX_CACHED_HDR)
        len = MAX_CACHED_HDR;
    
    __u32 len_64 = len/8;
    __u32 rest = len%8;

    bpf_spin_lock(&cached->lock);
    // MAX_CACHED_HEADER = 2104 (sizeof(ipv6hdr) + sizeof(ipv6_sr_hdr) + max(segList)) bytes
    // copy cached header in 8-byte blocks fore efficiency
    for(__u32 i = 0; i < MAX_CACHED_HDR / 8; i++){
        if(i >= len_64) break;
        if((void *)(dst + i + 1) > end){
            bpf_spin_unlock(&cached->lock);
            return -1;
        }

        __builtin_memcpy((void *)(dst + i), (void *)((__u64 *)cached->bytes + i), 8);
    }

    // copy the leftover bytes indiviudally
    #pragma unroll
    for(__u32 i = 0; i < 8; i++){
        if(i >= rest) break;
        char *dst_bytes = (char *) (dst + len_64); 
        char *cached_bytes = (char *) (dst + len_64); 
        if((void *)(dst_bytes + i + 1) > end){
            bpf_spin_unlock(&cached->lock);
            return -1;
        }
        dst_bytes[i] = cached_bytes[i];
        //__builtin_memcpy((void *)((char *)dst + len_64*8 + i), (void *)((char *)cached->bytes + len_64*8 + i), 1);
    }
    bpf_spin_unlock(&cached->lock);

    return 0;
}

static __always_inline
int update_ethhdr_encap(struct ethhdr *eth, void *end)
{
    if((void *)(eth + 1) > end)
        return -1;
    
    __u32 src_key = 2*N;
    __u32 dst_key = 2*N + 1;

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
    if((void *)((char *)srh + 1) > end)
        return -1;
    
    srh->nexthdr = version;
    switch (version)
    {
    case 4:
        srh->nexthdr = IPPROTO_IPIP;
        break;
    case 6:
        srh->nexthdr = IPPROTO_IPV6;
    default:
        break;
    }
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

    if(update_srh(p, end) < 0)
        return XDP_DROP;

    struct cached_outer *cached;
    cached = bpf_map_lookup_elem(&srh_table, &lane);
    if(!cached)
        return XDP_DROP;

    if(update_cached_header(cached, (char *)p->ip6, &lane, p->outer_len, end) < 0)
        return XDP_DROP;

    if(cp_ethhdr_in_front_of_inner(p))

    if(update_eth_hdr(p->eth, end, lane) < 0)
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

    // always use the one cached entry for dyanmic proxying
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

char LICENSE[] SEC("license") = "GPL";