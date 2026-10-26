// SPDX-License-Identifier: GPL-2.0
/*
 * srv6_cap.ebpf.c - per-target SRv6 proxy for targets that don't speak SRv6.
 *
 * (WIP)
 * 
 */
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/in6.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/seg6.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>


SEC("xdp")
int srv6_cap(struct xdp_md *ctx)
{
	return XDP_PASS;
}

char LICENSE[] SEC("license") = "GPL";
