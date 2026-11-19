#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/seg6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/*
CONSTANTS
*/

/* Values from include/uapi/linux/bpf.h (enum of BPF_F_ADJ_ROOM_*). */
#define PROXY_ADJ_ROOM_FIXED_GSO       (1ULL << 0)
#define PROXY_ADJ_ROOM_ENCAP_L3_IPV6   (1ULL << 2)
#define PROXY_ADJ_ROOM_NO_CSUM_RESET   (1ULL << 5)
#define PROXY_ADJ_ROOM_DECAP_L3_IPV4   (1ULL << 7)   /* kernel >= 6.3 */
#define PROXY_ADJ_ROOM_DECAP_L3_IPV6   (1ULL << 8)   /* kernel >= 6.3 */

#define MAX_LANES 8
#define MAX_SEGMENTS 128
#define MAX_COMP_SEGMENTS 128
#define MAX_SEGLIST_BYTES MAX_SEGMENTS*16 // 256 (8-bit hdrextLen field) 8-byte blocks -> excluding the first 8 bytes: only segs and tlvs!
#define RH_SIZE 8
#define MAX_SRH_LEN MAX_SEGLIST_BYTES + RH_SIZE// 256 (8-bit hdrextLen field) 8-byte blocks + the first 8 octets
#define MAX_CACHED_HDR (sizeof(struct ipv6hdr) + MAX_SRH_LEN) // account for routing header

#define MAX_L3_HDR_BYTES 40 // max(max_len(ipv6_hdr), max_len(ipv4_hdr))
#define MAX_L4_HDR_BYTES 20 // max(max_len(udp_hdr), max_len(tcp_hdr))
#define MAX_SRH_KEY_LEN MAX_L3_HDR_BYTES + MAX_L4_HDR_BYTES
#define MAX_SRH_CACHE_ENTRIES 128
#define L3_PROTOCOLS_SUPPORTED 2
#define L4_PROTOCOLS_SUPPORTED 2
#define MAX_L3_L4_COMBINATIONS L3_PROTOCOLS_SUPPORTED * L4_PROTOCOLS_SUPPORTED // #supported_l3_protos * #supported_l4_protos
#define MAX_FIXED_HDR_LEN_GLOBAL 40 // max(MAX_L3_HDR_BYTES, MAX_L4_HDR_BYTES)
#define L3_KEY_SIZE MAX_L3_HDR_BYTES
#define L4_KEY_SIZE MAX_L4_HDR_BYTES
#define MAX_ETH_L3_LEN_ENCAP (sizeof(struct ethhdr) + 40) // inner header is at most ETH plus 40 for IPv6 (ignoring IPv4 options)
#define MAX_ETH_L3_LEN_DECAP (sizeof(struct ethhdr) + sizeof(struct ipv6hdr) + sizeof(struct ipv6_sr_hdr) + MAX_SEGLIST_BYTES + 40)

/*
CONSTANTS TO BE OVERWRITTEN AT LOAD TIME
*/
const volatile __u32 n = MAX_LANES;


/*
DEFINE BARRIER_VAR in case libbpf helpers are not available
*/
#ifndef barrier_var
#define barrier_var(var) asm volatile("" : "+r"(var))
#endif

/*
STRUCTS
*/

struct cached_outer {
    struct bpf_spin_lock lock;
    __u16 len;
    __u16 pad;
    __u8 bytes[MAX_CACHED_HDR];
};

struct cached_outer_seq {
    __u32 seq;
    __u16 len;
    __u16 pad;
    __u8 bytes[MAX_CACHED_HDR];
    __u64 last_seen;
};

struct parsed_srh {
    struct ethhdr *eth;
    struct ipv6hdr *ip6;
    struct ipv6_sr_hdr *srh;
    __u32 srh_len;
    __u32 outer_len;
    __u16 next_proto;
    __u32 n_segs;
};

struct mac_entry {
    __u8 addr[6];
    __u8 pad[2];
};

/*
Lane-Based specific Structs
*/

struct sfc_t {
    struct bpf_spin_lock lock;
    __u64 last_seen; // jiffies at packet reception
    struct in6_addr segments[MAX_SEGMENTS];
    __u32 n_segs;
};

struct lane_meta {
    __u32 lane;
} __attribute__((aligned(4)));

/*
Field-Based specific Structs
*/

struct srh_key {
    __u8 l3[L3_KEY_SIZE];
    __u8 l4[L4_KEY_SIZE];
};

struct l3_mask_t {
    __u8 bytes[MAX_L3_HDR_BYTES];
};

struct l4_mask_t {
    __u8 bytes[MAX_L4_HDR_BYTES];
};

struct inner_l3_hdr {
    struct ipv6hdr *ip6;
    struct iphdr *ip4;
};

struct cached_outer_mock {
    __u8 bytes[sizeof(struct cached_outer)];
};

struct scratch_outer {
    __u16 len;
    __u8 bytes[MAX_CACHED_HDR];
};