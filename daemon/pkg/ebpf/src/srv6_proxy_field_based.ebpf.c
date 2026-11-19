#include "proxy.h"

/*
This proxy version is based on immutable header fields,
which uniquely identify a service chain.
The key corresponding to one sfc (and stored SRH) are computed based on a mask_map for each supported Layer.
Currently, support for L3 (IP6, IP4) and L4 (TCP, UDP) is built.
The proxy relies on two mask maps (mask_table_l3, mask_table_l4).
The user space loader configures and calculates the masks based on state in the targets spec in the K8s api.
The ebpf program just loads and uses the maps to compute a key to index into the srh_table.
We use optimistic locking here to enable the use of a LRU hash map to offload eviction etc. to the kernel.
*/

#define MAX_READ_RETRIES 4
#define REFRESH_RATE 1024 // refresh cache every ~second (1024 jiffies)
// use to keep compiler from merging variables which might have been changed by concurrent writers
#define READ_ONCE(x) (*(volatile typeof(x) *)&(x))

// kernel must be 5.18+
#ifndef bpf_xdp_load_bytes
static long (*bpf_xdp_load_bytes)(struct xdp_md *xdp_md, __u32 offset, void *buf, __u32 len) = (void *) 189;
static long (*bpf_xdp_store_bytes)(struct xdp_md *xdp_md, __u32 offset, void *buf, __u32 len) = (void *) 190;
#endif

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_SRH_CACHE_ENTRIES);
    __type(key, struct srh_key);
    __type(value, struct cached_outer_seq);
} srh_table SEC(".maps");

// not modified by xdp programs
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 4); 
    __type(key, __u32);
    __type(value, struct mac_entry);
} mac_addr_table SEC(".maps");

// not modified by xdp programs
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32);
    __type(value, __u32);
} link_index_table SEC(".maps");

// not modified by xdp programs
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 2);
    __type(key, __u32); 
    __type(value, struct l3_mask_t); 
} mask_table_l3 SEC(".maps");

// not modified by xdp programs
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 2);
    __type(key, __u32); 
    __type(value, struct l4_mask_t); 
} mask_table_l4 SEC(".maps");

// is only read -> no locking required
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct cached_outer_seq);
} scratch_table SEC(".maps");

/*
* SHARED FUNCTIONS
*/
static __always_inline
int parse_srh(struct parsed_srh *p, void *start, void *end)
{
    p->eth = start;
    if((void *)(p->eth + 1) > end)
        return -1;

    p->ip6 = (void *)(p->eth + 1);
    if((void *)(p->ip6 + 1) > end)
        return -1;
    
    if(p->ip6->version != 6 || p->ip6->nexthdr != 43){
        return 1;
    }

    p->srh = (void *)(p->ip6 + 1);
    if((void *)((char *)p->srh + 8) > end)  
        return -1;
        
    if(p->srh->type != 4) return 1;

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
    
    p->n_segs = (__u32)last_index + 1;
    
    return 0;
}

static __always_inline int mask_n(const __u8 *hdr, const __u8 *mask, __u8 *dst, void *end, const __u32 n)
{
    if ((const void *)(hdr + n) > end)
        return -1;
    barrier_var(hdr);
    #pragma unroll
    for (__u32 i = 0; i < n; i++)
        dst[i] = mask[i] & hdr[i];
    return 0;
}

static __always_inline
int compute_srh_key(struct srh_key *key, void *end, char *inner, __u32 *ver)
{
    if ((void *)(inner + 1) > end) return -1;
    __u32 version = *(__u8 *)inner >> 4;
    *ver = version;

    struct l3_mask_t *m3 = bpf_map_lookup_elem(&mask_table_l3, &version);
    if (!m3) return -1;

    __u32 proto, l3_len;
    if (version == 4) {
        struct iphdr *ip4 = (struct iphdr *)inner;
        if ((void *)(ip4 + 1) > end) return -1;
        l3_len = ip4->ihl * 4;
        if (l3_len < 20) return -1;
        proto = ip4->protocol;
        if (proto != IPPROTO_TCP && proto != IPPROTO_UDP) return 1;
        if (mask_n((__u8 *)inner, m3->bytes, key->l3, end, 20) < 0) return -1;
    } else if (version == 6) {
        struct ipv6hdr *ip6 = (struct ipv6hdr *)inner;
        if ((void *)(ip6 + 1) > end) return -1;
        l3_len = 40;
        proto = ip6->nexthdr;
        if (proto != IPPROTO_TCP && proto != IPPROTO_UDP) return 1;
        if (mask_n((__u8 *)inner, m3->bytes, key->l3, end, 40) < 0) return -1;
    } else {
        return -1;
    }

    struct l4_mask_t *m4 = bpf_map_lookup_elem(&mask_table_l4, &proto);
    if (!m4) return -1;

    if(l3_len > 60) return -1;
    __u8 *l4 = (__u8 *)inner + l3_len;
    barrier_var(l4);
    if ((void *)(l4 + 20) > end) return -1;
    if (proto == IPPROTO_TCP)
        return mask_n(l4, m4->bytes, key->l4, end, 20) < 0 ? -1 : 0;
    return mask_n(l4, m4->bytes, key->l4, end, 8) < 0 ? -1 : 0;
}

/*
* DECAP FUNCTIONS
*/

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
int update_cached_header(struct cached_outer_seq *cached, char *hdr, __u32 len, void *end, struct xdp_md *ctx)
{
    if(len < 40 || len > MAX_CACHED_HDR)
        return -1;
    barrier_var(len);
    if(sizeof(cached->bytes) < len)
        return -1;
    if((void *)(hdr + len) > end)
        return -1;

    __u32 s = READ_ONCE(cached->seq);
    if(s % 2 != 0)
        return 0; // skip writing because entry is being actively written
    // try to claim seq for writing
    __u32 old_s = __sync_val_compare_and_swap(&cached->seq, s, s+1);
    // check if cached->seq has been changed in the meantime (CAS failed)
    if(old_s != s)
        return 0;
    
    // update cached entry
    if(bpf_xdp_load_bytes(ctx, ETH_HLEN, (void *)cached->bytes, len) < 0){
        __sync_fetch_and_add(&cached->seq, 1);
        return -1;
    }
    cached->len = len;
    cached->last_seen = bpf_jiffies64(); 
    
    __sync_fetch_and_add(&cached->seq, 1);
    return 0;
}

/*
ENCAP FUNCTIONS
*/

// copies a cached header from an eBPF map (src) to the xdp_buffer (dst)
// retries MAX_READ_RETRIES times to read the entry and drops otherwise
static __always_inline
int copy_cached_to_buffer(struct ethhdr *eth, struct cached_outer_seq *cached, void *end, __u32 len, struct xdp_md *ctx)
{
    if((void *)(eth + 1) > end)
        return -1;

    if(len > MAX_CACHED_HDR)
        len = MAX_CACHED_HDR;
    if(len < 40)
        return -1;
    barrier_var(len);

    for(int r = 1; r <= MAX_READ_RETRIES; r++){
        __u32 seq = READ_ONCE(cached->seq);
        // dont even attempt to read if seq unequal
        if(seq & 1)
            continue;
        if (READ_ONCE(cached->len) != len) // if cached->len was changed after making room for the header -> drop
            return -1;

        if(bpf_xdp_store_bytes(ctx, ETH_HLEN, cached->bytes, len) < 0)
            return -1;

        // check if seq stayed the same during read
        if(READ_ONCE(cached->seq) == seq){
            return 0;
        }
        if(r == MAX_READ_RETRIES)
            return -1; // all tries failed -> drop
    }

    return -1;
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
    
    __builtin_memcpy((void *)eth->h_source, (void *)src->addr, sizeof(eth->h_source));
    __builtin_memcpy((void *)eth->h_dest, (void *)dst->addr, sizeof(eth->h_dest));

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


static __always_inline
int update_eth_hdr(void *start, void *end)
{
    struct ethhdr *eth = start;
    if((void *)((char *)eth + sizeof(struct ethhdr)) > end)
        return -1;

    __u32 src_key = 0;
    __u32 dst_key = 1;
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

/*
XDP PROGRAMS
*/

SEC("xdp")
int proxy_ingress(struct xdp_md *ctx)
{   
    void *start = (void *)(long)ctx->data;
    void *end = (void *)(long)ctx->data_end;
    
    struct parsed_srh p = {};
    int ret = parse_srh(&p, start, end);
    if(ret < 0) return XDP_DROP;
    if(ret > 0) return XDP_PASS;

    char *inner_start = (char *)((char *)p.srh + p.srh_len);
    struct srh_key key = {0};

    __u32 inner_l3_version = -1;
    ret = compute_srh_key(&key, end, inner_start, &inner_l3_version);
    if(ret < 0) return XDP_DROP;
    if(ret > 0) return XDP_PASS;
    

    if(update_srh(&p, end) < 0)
        return XDP_DROP;

    struct cached_outer_seq *cached;
    cached = bpf_map_lookup_elem(&srh_table, &key);
    if(!cached){
        // not initialized -> zero init from scratch map to save stack space
        __u32 zero_key = 0;
        struct cached_outer_mock *zeroed = bpf_map_lookup_elem(&scratch_table, &zero_key);
        if(!zeroed)
            return XDP_DROP;
        bpf_map_update_elem(&srh_table, &key, zeroed->bytes, BPF_NOEXIST);
        cached = bpf_map_lookup_elem(&srh_table, &key);
        if(!cached)
            return XDP_DROP;
    }

    // only write new cache entry if the segment list changed
    /* if(!segs_equal(cached, &p, end)){
    } */
    // update only, if last update for this entry is older than REFRESH_RATE
    if(bpf_jiffies64() - cached->last_seen > REFRESH_RATE){
        if(update_cached_header(cached, (char *)p.ip6, p.outer_len, end, ctx) < 0)
            return XDP_DROP;
    }

    struct ethhdr *new_eth = (struct ethhdr *)((char *)p.eth + p.outer_len);
    if((void *)(new_eth + 1) > end)
        return XDP_DROP;
    if((void *)(p.eth + 1) > end)
        return XDP_DROP;
    __builtin_memcpy((void *)new_eth, (void *)p.eth, sizeof(struct ethhdr));

    if(update_eth_hdr(new_eth, end) < 0)
        return XDP_DROP;

    if(bpf_xdp_adjust_head(ctx, p.outer_len) < 0)
        return XDP_DROP;

    // fetch linkIndex of outgoing interface, per convention at index 0
    __u32 link_key = 0;
    __u32 *link_index = bpf_map_lookup_elem(&link_index_table, &link_key);
    
    if(!link_index || !*link_index)
        return XDP_DROP;
    
    return bpf_redirect(*link_index, 0);
}

SEC("xdp")
int proxy_egress(struct xdp_md *ctx)
{   
    void *start = (void *)(long)ctx->data;
    void *end = (void *)(long)ctx->data_end;

    struct ethhdr *eth = start;
    if((void *)(eth + 1) > end)
        return XDP_DROP;

    char *inner_start = (char *)(eth + 1);

    if((void *)(inner_start + 1) > end)
        return XDP_PASS;
    __u8 version = *(inner_start) >> 4;
    if(version != 4 && version != 6)
        return XDP_PASS;
    
    if(version == 4){
        if(eth->h_proto != bpf_htons(ETH_P_IP)){
            return XDP_PASS;
        }
    } else {
        if(eth->h_proto != bpf_htons(ETH_P_IPV6))
            return XDP_PASS;
    }

    struct srh_key key = {0};
    __u32 inner_l3_version = -1;
    int ret = compute_srh_key(&key, end, inner_start, &inner_l3_version);
    if(ret > 0) return XDP_PASS;
    if (ret < 0) return XDP_DROP;
    
    struct cached_outer_seq *cached = bpf_map_lookup_elem(&srh_table, &key);
    if(!cached){
        return XDP_DROP; // if no entry is found, something must have gone wrong -> drop the packet
    }

    __u32 len = 0;
    int ok = 0;

    for (int r = 0; r < MAX_READ_RETRIES; r++) {
        __u32 seq = READ_ONCE(cached->seq);
        if (seq & 1)
            continue;
        len = READ_ONCE(cached->len);
        if (READ_ONCE(cached->seq) != seq)
            continue;
        ok = 1;
        break;
    }

    if (!ok || len < sizeof(struct ipv6hdr) + 8 || len > MAX_CACHED_HDR) return XDP_DROP;
    
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
    
    if(copy_cached_to_buffer(eth, cached, end, len, ctx) < 0)
        return XDP_DROP;

    start = (void *)(long)ctx->data;
    end   = (void *)(long)ctx->data_end;
    eth   = start;
    if(update_nxthdr(eth, end, inner_l3_version) < 0)
        return XDP_DROP;

    // lookup index of dispatcher veth
    __u32 link_index_key = 2; // per convention the dispatcher veths idx is at index 2
    __u32 *link_index = bpf_map_lookup_elem(&link_index_table, &link_index_key);
    if(!link_index)
        return XDP_DROP;
    return bpf_redirect(*link_index, 0);
}

char LICENSE[] SEC("license") = "GPL";