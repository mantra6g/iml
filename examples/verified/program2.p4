/* -*- P4_16 -*- */
#include <core.p4>
#include <v1model.p4>

#define METADATAFIELDSRV6 bit<128> next_sid;
#define SRH_SID_MAX 4

#define SRVHEADERS header ipv6_srh_h { \
    bit<8> next_hdr; \
    bit<8> hdr_ext_len; \
    bit<8> routing_type; \
    bit<8> seg_left; \
    bit<8> last_entry; \
    bit<8> flags; \
    bit<12> tag; \
    bit<4> gtpMessageType; \
} \
header ipv6_srh_segment_h { \
    bit<128> sid; \
}

#define SRVHEADERINSTANCES ipv6_srh_h srh; \
ipv6_srh_segment_h srh_sid_0; \
ipv6_srh_segment_h srh_sid_1; \
ipv6_srh_segment_h srh_sid_2; \
ipv6_srh_segment_h srh_sid_3;

#define SRVSTATES(pkt,hdr) state parse_srh { \
        pkt.extract(hdr.srh); \
        transition parse_srh_sid_0; \
    } \
    state parse_srh_sid_0 {                \
        pkt.extract(hdr.srh_sid_0);         \
        transition select(hdr.srh.last_entry) {  \
            0 : parse_post_srh;       \
            default : parse_srh_sid_1;     \
        }                                       \
    } \
    state parse_srh_sid_1 {                \
        pkt.extract(hdr.srh_sid_1);         \
        transition select(hdr.srh.last_entry) {  \
            1 : parse_post_srh;       \
            default : parse_srh_sid_2;     \
        }                                       \
    } \
    state parse_srh_sid_2 {                \
        pkt.extract(hdr.srh_sid_2);         \
        transition select(hdr.srh.last_entry) {  \
            2 : parse_post_srh;       \
            default : parse_srh_sid_3;     \
        }                                      \
    } \
    state parse_srh_sid_3 { \
        pkt.extract(hdr.srh_sid_3); \
        transition select(hdr.srh.last_entry) { \
            3 : parse_post_srh; \
        } \
    }

#define SRVEMIT(pkt,hdr) pkt.emit(hdr.srh); \
pkt.emit(hdr.srh_sid_0); \
pkt.emit(hdr.srh_sid_1); \
pkt.emit(hdr.srh_sid_2); \
pkt.emit(hdr.srh_sid_3);

const bit<16> TYPE_IPV4 = 0x800;

/*************************************************************************
*********************** H E A D E R S  ***********************************
*************************************************************************/

typedef bit<9>  egressSpec_t;
typedef bit<48> macAddr_t;
typedef bit<32> ip4Addr_t;

SRVHEADERS

header ethernet_t {
    macAddr_t dstAddr;
    macAddr_t srcAddr;
    bit<16>   etherType;
}

header ipv4_t {
    bit<4>    version;
    bit<4>    ihl;
    bit<8>    diffserv;
    bit<16>   totalLen;
    bit<16>   identification;
    bit<3>    flags;
    bit<13>   fragOffset;
    bit<8>    ttl;
    bit<8>    protocol;
    bit<16>   hdrChecksum;
    ip4Addr_t srcAddr;
    ip4Addr_t dstAddr;
}
header ipv6_t{
	bit<4> version;
	bit<8> trafficClass;
	bit<20> flowLabel;
	bit<16> payLoadLen;
	bit<8> nextHdr;
	bit<8> hopLimit;
	bit<128> srcAddr;
	bit<128> dstAddr;
}

// New header type for transfering needed data
header dissaggregation_header_t {
    egressSpec_t port;
    bit<7> padding;
}


struct metadata {
    METADATAFIELDSRV6
    egressSpec_t port; 
}

struct headers {
    SRVHEADERINSTANCES
    dissaggregation_header_t  dissaggregation_header; // New header for transfering needed data
    ethernet_t   ethernet;
    ipv6_t       ipv6;
}

/*************************************************************************
*********************** P A R S E R  ***********************************
*************************************************************************/

parser MyParser(packet_in packet,
                out headers hdr,
                inout metadata meta,
                inout standard_metadata_t standard_metadata) {

    SRVSTATES(packet,hdr)
    state start {
        packet.extract(hdr.ethernet);
        packet.extract(hdr.ipv6);
        transition parse_srh;
    }

    state parse_post_srh{
        transition accept;
    }


}

/*************************************************************************
************   C H E C K S U M    V E R I F I C A T I O N   *************
*************************************************************************/

control MyVerifyChecksum(inout headers hdr, inout metadata meta) {   
    apply {  }
}


/*************************************************************************
**************  I N G R E S S   P R O C E S S I N G   *******************
*************************************************************************/

control MyIngress(inout headers hdr,
                  inout metadata meta,
                  inout standard_metadata_t standard_metadata) {
    counter(1, CounterType.packets) part_2c;
    register<bit<32>>(1024) reg;
    action set_port() {
        standard_metadata.egress_spec = 2;  
    }
    
    
    table ipv6_lpm2 {
        key = {
            hdr.ipv6.dstAddr: lpm;
        }
        actions = {
            set_port;
            NoAction;
        }
        size = 1024;
        default_action = NoAction();
    }

    bit<32> a;
    
    apply {		
        part_2c.count(0);
        reg.write(0,hdr.ipv6.dstAddr[31:0]);
        ipv6_lpm2.apply();
        reg.read(a, 1);
        a = a + 1;
        reg.write(1, a);    
        standard_metadata.egress_spec = standard_metadata.ingress_port;
    }
}

/*************************************************************************
****************  E G R E S S   P R O C E S S I N G   *******************
*************************************************************************/

control MyEgress(inout headers hdr,
                 inout metadata meta,
                 inout standard_metadata_t standard_metadata) {
    apply {  }
}

/*************************************************************************
*************   C H E C K S U M    C O M P U T A T I O N   **************
*************************************************************************/

control MyComputeChecksum(inout headers  hdr, inout metadata meta) {
     apply { }
}

/*************************************************************************
***********************  D E P A R S E R  *******************************
*************************************************************************/

control MyDeparser(packet_out packet, in headers hdr) {
    apply {
        packet.emit(hdr.ethernet);
        packet.emit(hdr.ipv6);
        SRVEMIT(packet,hdr)
    }
}

/*************************************************************************
***********************  S W I T C H  *******************************
*************************************************************************/

V1Switch(
MyParser(),
MyVerifyChecksum(),
MyIngress(),
MyEgress(),
MyComputeChecksum(),
MyDeparser()
) main;
