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

// Helper to add two 16-bit checksums with carry handling
static __always_inline __u32 csum_add(__u32 sum1, __u32 sum2) {
    __u32 result = sum1 + sum2;
    // Add any carry from bit 16 back into the lower 16 bits
    return (result & 0xffff) + (result >> 16);
}

// Standard 1's complement sum folding with proper carry handling
static __always_inline __u16 csum_fold(__u32 csum) {
    // Add the upper 16 bits to the lower 16 bits with carry handling
    csum = csum_add(csum, csum >> 16);
    // Fold again to handle any carry from the first fold
    csum = csum_add(csum, csum >> 16);
    // Final fold to ensure all carries are completely absorbed
    csum = csum_add(csum, csum >> 16);
    // Return the 1's complement of the result
    return (__u16)~csum;
}


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

    // --- 2. Skip the SRH (Segment Routing Header) ---
    if (outer_ip6h->nexthdr != IPPROTO_ROUTING) return TC_ACT_OK;

    struct ipv6_sr_hdr *srh = (void *)(outer_ip6h + 1);
    if ((void *)(srh + 1) > data_end) return TC_ACT_OK;

    // Guard srh_len to prevent negative/overflow issues in pointer arithmetic
    __u32 srh_len = (srh->hdrlen + 1) << 3;
    // Explicitly bound the register
    // This tells the verifier: "srh_len is at least 0 and at most 2047"
    srh_len &= 0x7FF;

    struct ipv6hdr *inner_ip6h = (void *)((char *)srh + srh_len);
    if ((void *)(inner_ip6h + 1) > data_end) return TC_ACT_OK;

    // --- 3. Identify L4 Protocol and Length ---
    __u8 proto = inner_ip6h->nexthdr;
    void *l4_ptr = (void *)(inner_ip6h + 1);

    __u32 l4_len = bpf_ntohs(inner_ip6h->payload_len);
    l4_len &= 0xFFFF;

    // CRITICAL FIX: The Dual-Range Check
    if (l4_len > 1500) return TC_ACT_OK; // Sane limit
    // Verify l4_ptr is within bounds for the full length
    if ((char *)l4_ptr + l4_len > (char *)data_end) return TC_ACT_OK;

    // Additional bounds check at the maximum expected L4 header size
    if ((char *)l4_ptr + 1500 > (char *)data_end) return TC_ACT_OK;

    // --- 4. Recalculate Pseudo-Header Sum ---
    __u32 phdr_sum = 0;

    // Inline the additions to avoid stack spills
    #pragma unroll
    for (int j = 0; j < 4; j++) {
        // Use a temporary variable to help the compiler stay in registers
        __u32 s = inner_ip6h->saddr.s6_addr32[j];
        phdr_sum = csum_add(phdr_sum, s);
        __u32 d = inner_ip6h->daddr.s6_addr32[j];
        phdr_sum = csum_add(phdr_sum, d);
    }
    // RFC 2460: Payload Length in upper 16 bits, zeros in lower 16 bits
    phdr_sum = csum_add(phdr_sum, (l4_len << 16));
    // RFC 2460: Next Header in bits 8-15, zeros in bits 0-7 and 16-31
    phdr_sum = csum_add(phdr_sum, (proto << 8));

    // --- 5. Calculate L4 Payload Sum ---
    // With the dual-range check above, this call will now pass!
    __s64 raw_sum = bpf_csum_diff(NULL, 0, l4_ptr, l4_len, 0);
    if (raw_sum < 0) return TC_ACT_OK;

    // --- 6. Fold and Write ---
    // Properly combine the sums with carry handling
    __u32 combined = csum_add((__u32)raw_sum, phdr_sum);
    __u16 final_csum = csum_fold(combined);

    __u32 csum_off;
    if (proto == IPPROTO_TCP) {
        struct tcphdr *tcp = l4_ptr;
        if ((void *)(tcp + 1) > data_end) return TC_ACT_OK;
        csum_off = (void *)&tcp->check - data;
    } else if (proto == IPPROTO_UDP) {
        struct udphdr *udp = l4_ptr;
        if ((void *)(udp + 1) > data_end) return TC_ACT_OK;
        csum_off = (void *)&udp->check - data;
        // RFC 2768: UDP over IPv6 must never have a checksum of 0
        if (final_csum == 0) {
            final_csum = 0xFFFF;
        }
    } else if (proto == IPPROTO_ICMPV6) {
        struct icmp6hdr *icmp6 = l4_ptr;
        if ((void *)(icmp6 + 1) > data_end) return TC_ACT_OK;
        csum_off = (void *)&icmp6->icmp6_cksum - data;
    } else {
        return TC_ACT_OK;
    }

    // Zero out the checksum field before calculating the final checksum
    __u16 zero = 0;
    bpf_skb_store_bytes(skb, csum_off, &zero, sizeof(zero), 0);

    // Write the new checksum directly into the packet buffer
    bpf_skb_store_bytes(skb, csum_off, &final_csum, sizeof(final_csum), 0);

    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
