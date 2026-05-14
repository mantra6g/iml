// RIFO: Full P4 Implementation with Score-based Admission Logic

#include <core.p4>
#include <v1model.p4>

const bit<16> ETYPE_IPV4 = 0x0800;
const bit<16> ETYPE_IPV6 = 0x86DD;

typedef bit<9>  egressSpec_t;
typedef bit<48> macAddr_t;
typedef bit<32> ip4Addr_t;

const bit<32> B = 100; // set to the max. queue lenght in number of packets
const bit<32> kB = 30;
const bit<16> T = 10000;

register<bit<32>>(1) reg_queue_length;

header ethernet_t {
    bit<48> dstAddr;
    bit<48> srcAddr;
    bit<16> etherType;
}

header ipv4_t {
    bit<4>  version;
    bit<4>  ihl;
    bit<6>  dscp;
    bit<2>  ecn;
    bit<16> totalLen;
    bit<16> identification;
    bit<3>  flags;
    bit<13> fragOffset;
    bit<8>  ttl;
    bit<8>  protocol;
    bit<16> hdrChecksum;
    bit<32> srcAddr;
    bit<32> dstAddr;
}

header ipv6_t {
    bit<4>   version;
    bit<6>   dscp;
    bit<2>   ecn;
    bit<20>  flowLabel;
    bit<16>  payloadLen;
    bit<8>   nextHdr;
    bit<8>   hopLimit;
    bit<128> srcAddr;
    bit<128> dstAddr;
}

header srv6_h {
	bit<8> next_hdr;
	bit<8> hdr_ext_len;
	bit<8> routing_type;
	bit<8> segments_left;
	bit<8> first_segment;
	bit<8> flags;
	bit<16> tag;
}

header segment_h {
	bit<128> segment;
}

struct headers {
    ethernet_t ethernet;
    ipv4_t ipv4;
    ipv6_t ipv6;
    //srv6_h srv6;
    //segment_h segment;
}

struct metadata {
    bit<16> rank;
}

parser MyParser(packet_in packet, out headers hdr, inout metadata meta, inout standard_metadata_t standard_metadata) {
    state start {
        packet.extract(hdr.ethernet);
        transition select(hdr.ethernet.etherType) {
            ETYPE_IPV4: parse_ipv4;
            ETYPE_IPV6: parse_ipv6;
            default: accept;
        }
    }
    state parse_ipv4 {
        packet.extract(hdr.ipv4);
        meta.rank = (bit<16>)hdr.ipv4.dscp;
        transition accept;
    }
    state parse_ipv6 {
        packet.extract(hdr.ipv6);
        meta.rank = (bit<16>)hdr.ipv6.dscp;
        transition accept;
    }

}

control MyIngress(inout headers hdr, inout metadata meta, inout standard_metadata_t standard_metadata) {

    register<bit<16>>(1) reg_min;
    register<bit<16>>(1) reg_max;
    register<bit<16>>(1) reg_count;
    register<bit<16>>(1) reg_threshold;

    const bit<16> INIT_MIN = 0xFFFF;
    const bit<16> INIT_MAX = 0x0000;

    action update_min(bit<16> rank) {
        reg_min.write(0, rank);
    }

    action update_max(bit<16> rank) {
        reg_max.write(0, rank);
    }

    action reset_min_max(bit<16> rank) {
        reg_min.write(0, rank);
        reg_max.write(0, rank);
        reg_count.write(0, 1);
    }

    action increment_counter() {
        bit<16> c;
        reg_count.read(c, 0);
        reg_count.write(0, c + 1);
    }

    action drop() {
        mark_to_drop(standard_metadata);
    }

    //action increment_queue() {
    //    bit<8> len;
    //    queue_length.read(len, 0);
    //    queue_length.write(0, len + 1);
    //}

    action forward(egressSpec_t port) {
        //increment_queue();
        standard_metadata.egress_spec = port;
    }

    table mac_forward {
        key = {
            hdr.ethernet.dstAddr: exact;
        }
        actions = {
            forward;
            drop;
            NoAction;
        }
        size = 1024;
    }

    apply {

        if (standard_metadata.ingress_port == 2) {
            if (hdr.ipv4.dstAddr == 0x0A000001) {
                standard_metadata.egress_spec = 1;
            } else {
                standard_metadata.egress_spec = 3;
            }
            return;
        }

        if (meta.rank == 0) {
            meta.rank = 7;
        }

        bit<1> forward_packet = 0;
        bit<1> rank_valid = 0;


        if (hdr.ipv4.isValid()) {
            rank_valid = 1;
        } else if (hdr.ipv6.isValid()) {
            rank_valid = 1;
        }

        if (rank_valid == 0) {
            standard_metadata.egress_spec = 2;
            return;
        }

        bit<16> count;
        bit<16> threshold;
        reg_count.read(count, 0);
        reg_threshold.read(threshold, 0);


        if (count == 0) {
            reg_min.write(0, INIT_MIN);
            reg_max.write(0, INIT_MAX);
        }

        bit<16> min_rank;
        bit<16> max_rank;
        reg_min.read(min_rank, 0);
        reg_max.read(max_rank, 0);

        if (count >= T) {
            reset_min_max(meta.rank);
        } else {
            if (meta.rank < min_rank) {
                update_min(meta.rank);
            }
            if (meta.rank > max_rank) {
                update_max(meta.rank);
            }
            increment_counter();
        }

        reg_min.read(min_rank, 0);
        reg_max.read(max_rank, 0);

        if (max_rank == min_rank) {
            forward_packet = 1;
        } else {

            bit<32> queue_len;
            reg_queue_length.read(queue_len, 0);
            bit<32> available = (bit<32>)B - queue_len;
            bit<16> rank_diff = meta.rank - min_rank;
            bit<16> range_val = max_rank - min_rank;

            bit<32> rank_expr = (bit<32>)rank_diff * (bit<32>)B;
            bit<32> range_expr = (bit<32>)range_val * (bit<32>)available;

            if (queue_len <= (bit<32>)kB) {
                forward_packet = 1;
            } else if (range_val != 0) {
                if (rank_expr <= range_expr) {
                    forward_packet = 1;
                }
            }
        }

        if (forward_packet == 1) {
            standard_metadata.egress_spec = 2;
        } else {
            drop();
        }
    }
}

control MyEgress(inout headers hdr, inout metadata meta, inout standard_metadata_t standard_metadata) {

    apply {
        reg_queue_length.write(0, (bit<32>)standard_metadata.enq_qdepth);
    }
}

control MyVerifyChecksum(inout headers hdr, inout metadata meta) {
    apply { }
}

control MyComputeChecksum(inout headers hdr, inout metadata meta) {
    apply { }
}

control MyDeparser(packet_out packet, in headers hdr) {
    apply {
        packet.emit(hdr.ethernet);
        packet.emit(hdr.ipv4);
        packet.emit(hdr.ipv6);
    }
}

V1Switch(
    MyParser(),
    MyVerifyChecksum(),
    MyIngress(),
    MyEgress(),
    MyComputeChecksum(),
    MyDeparser()
) main;
