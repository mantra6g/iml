#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/ipv6.h>
#include <linux/seg6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <linux/icmpv6.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

SEC("tc")
int force_recalc_srv6_csum(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    // --- 1. Find the Outer Header ---
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) return TC_ACT_OK;
    if (eth->h_proto != bpf_htons(ETH_P_IPV6)) return TC_ACT_OK;

    struct ipv6hdr *outer_ip6h = (void *)(eth + 1);
    if ((void *)(outer_ip6h + 1) > data_end) return TC_ACT_OK;

    // --- 2. Skip the SRH ---
    if (outer_ip6h->nexthdr != IPPROTO_ROUTING) return TC_ACT_OK;

    struct ipv6_sr_hdr *srh = (void *)(outer_ip6h + 1);
    if ((void *)(srh + 1) > data_end) return TC_ACT_OK;

    __u32 srh_len = (srh->hdrlen + 1) << 3;
    srh_len &= 0x7FF;

    struct ipv6hdr *inner_ip6h = (void *)((char *)srh + srh_len);
    if ((void *)(inner_ip6h + 1) > data_end) return TC_ACT_OK;

    // --- 3. Identify L4 Protocol and Length ---
    __u8 proto = inner_ip6h->nexthdr;
    void *l4_ptr = (void *)(inner_ip6h + 1);

    __u32 l4_len = bpf_ntohs(inner_ip6h->payload_len);
    l4_len &= 0xFFFF;

    if (l4_len > 1500) return TC_ACT_OK;
    if ((char *)l4_ptr + l4_len > (char *)data_end) return TC_ACT_OK;
    if ((char *)l4_ptr + 1500 > (char *)data_end) return TC_ACT_OK;

    // --- 4. Zero Checksum via Direct Pointers & Store Offset ---
    __u32 csum_off;

    // We zero the checksum directly in memory so bpf_csum_diff doesn't
    // absorb it. We do NOT use bpf_skb_store_bytes here because it would
    // invalidate l4_ptr and data_end before we run bpf_csum_diff.
    if (proto == IPPROTO_TCP) {
        struct tcphdr *tcp = l4_ptr;
        if ((void *)(tcp + 1) > data_end) return TC_ACT_OK;
        csum_off = (void *)&tcp->check - data;
        tcp->check = 0;
    } else if (proto == IPPROTO_UDP) {
        struct udphdr *udp = l4_ptr;
        if ((void *)(udp + 1) > data_end) return TC_ACT_OK;
        csum_off = (void *)&udp->check - data;
        udp->check = 0;
    } else if (proto == IPPROTO_ICMPV6) {
        struct icmp6hdr *icmp6 = l4_ptr;
        if ((void *)(icmp6 + 1) > data_end) return TC_ACT_OK;
        csum_off = (void *)&icmp6->icmp6_cksum - data;
        icmp6->icmp6_cksum = 0;
    } else {
        return TC_ACT_OK;
    }

    // --- 5. Recalculate Pseudo-Header Sum ---
    __u64 csum = 0;

    #pragma unroll
    for (int j = 0; j < 4; j++) {
        csum += inner_ip6h->saddr.s6_addr32[j];
        csum += inner_ip6h->daddr.s6_addr32[j];
    }

    // RFC 2460: 32-bit payload length and 32-bit protocol format
    csum += bpf_htonl(l4_len);
    csum += bpf_htonl(proto);

    // --- 6. Calculate L4 Payload Sum ---
    __s64 raw_sum = bpf_csum_diff(NULL, 0, l4_ptr, l4_len, 0);
    if (raw_sum < 0) return TC_ACT_OK;

    csum += raw_sum;

    // --- 7. Fold ---
    // Fold 64 → 32
    csum = (csum & 0xffffffff) + (csum >> 32);
    csum = (csum & 0xffffffff) + (csum >> 32);
    // Fold 32 → 16
    csum = (csum & 0xffff) + (csum >> 16);
    csum = (csum & 0xffff) + (csum >> 16);
    __u16 final_csum = ~(__u16)csum;

    if (proto == IPPROTO_UDP && final_csum == 0) {
        final_csum = 0xFFFF;
    }

    // --- 8. Write via Helper to Invalidate Kernel Checksum State ---
    // This safely modifies the packet AND forces skb->ip_summed = CHECKSUM_NONE
    bpf_skb_store_bytes(skb, csum_off, &final_csum, sizeof(final_csum), 0);

    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
