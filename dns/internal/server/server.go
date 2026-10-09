// Package server implements an authoritative DNS server for loom Services. Like the CoreDNS
// kubernetes plugin, it answers <name>.<namespace>.svc.<zone> with the cluster IPs of the
// loom Service <namespace>/<name>.
package server

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/miekg/dns"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

const (
	// serviceLabel is the label below the zone under which Services are published.
	serviceLabel = "svc"
	// udpBufferSize is the EDNS0 UDP payload size advertised in responses.
	udpBufferSize = 1232
	// lookupTimeout bounds a single lookup against the Service reader.
	lookupTimeout = 2 * time.Second
)

// Handler answers DNS queries for the zone it is authoritative for.
type Handler struct {
	reader client.Reader
	zone   string
	ttl    uint32
	serial uint32
	log    logr.Logger
}

// New returns a Handler that is authoritative for zone (e.g. "loom.local.") and reads loom
// Services from reader. Records are returned with the given TTL.
func New(reader client.Reader, zone string, ttl uint32, log logr.Logger) *Handler {
	return &Handler{
		reader: reader,
		zone:   dns.CanonicalName(zone),
		ttl:    ttl,
		serial: uint32(time.Now().Unix()),
		log:    log,
	}
}

// ServeDNS implements dns.Handler.
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = true

	switch {
	case r.Opcode != dns.OpcodeQuery:
		m.SetRcode(r, dns.RcodeNotImplemented)
	case len(r.Question) != 1:
		m.SetRcode(r, dns.RcodeFormatError)
	case r.Question[0].Qclass != dns.ClassINET && r.Question[0].Qclass != dns.ClassANY:
		m.SetRcode(r, dns.RcodeRefused)
	default:
		h.answer(m, r.Question[0])
	}

	size := dns.MaxMsgSize
	if opt := r.IsEdns0(); opt != nil {
		m.SetEdns0(udpBufferSize, opt.Do())
		if isUDP(w) {
			size = min(int(opt.UDPSize()), udpBufferSize)
		}
	} else if isUDP(w) {
		size = dns.MinMsgSize
	}
	m.Truncate(max(size, dns.MinMsgSize))

	if err := w.WriteMsg(m); err != nil {
		h.log.Error(err, "failed to write DNS response", "question", r.Question)
	}
}

// answer fills m with the response to q.
func (h *Handler) answer(m *dns.Msg, q dns.Question) {
	qname := dns.CanonicalName(q.Name)
	if !dns.IsSubDomain(h.zone, qname) {
		m.Rcode = dns.RcodeRefused
		return
	}
	m.Authoritative = true

	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()

	rcode, answer, err := h.resolve(ctx, qname, q.Qtype)
	if err != nil {
		h.log.Error(err, "failed to resolve query", "name", qname, "type", dns.TypeToString[q.Qtype])
		m.Authoritative = false
		m.Rcode = dns.RcodeServerFailure
		return
	}
	m.Rcode = rcode
	m.Answer = answer
	if len(answer) == 0 {
		// Negative answers (NXDOMAIN and NODATA) carry the zone SOA (RFC 2308).
		m.Ns = []dns.RR{h.soa()}
	}
}

// resolve returns the rcode and answer records for an in-zone name. An empty answer with
// RcodeSuccess is a NODATA response.
func (h *Handler) resolve(ctx context.Context, qname string, qtype uint16) (int, []dns.RR, error) {
	labels := dns.SplitDomainName(strings.TrimSuffix(qname, h.zone))

	switch len(labels) {
	case 0: // zone apex
		switch qtype {
		case dns.TypeSOA:
			return dns.RcodeSuccess, []dns.RR{h.soa()}, nil
		case dns.TypeNS:
			return dns.RcodeSuccess, []dns.RR{h.ns()}, nil
		case dns.TypeANY:
			return dns.RcodeSuccess, []dns.RR{h.soa(), h.ns()}, nil
		}
		return dns.RcodeSuccess, nil, nil

	case 1: // svc.<zone>
		if labels[0] != serviceLabel {
			return dns.RcodeNameError, nil, nil
		}
		return dns.RcodeSuccess, nil, nil

	case 2: // <namespace>.svc.<zone>
		namespace := labels[0]
		if labels[1] != serviceLabel || len(validation.IsDNS1123Label(namespace)) != 0 {
			return dns.RcodeNameError, nil, nil
		}
		exists, err := h.namespaceExists(ctx, namespace)
		if err != nil {
			return 0, nil, err
		}
		if !exists {
			return dns.RcodeNameError, nil, nil
		}
		return dns.RcodeSuccess, nil, nil

	case 3: // <name>.<namespace>.svc.<zone>
		name, namespace := labels[0], labels[1]
		if labels[2] != serviceLabel || len(validation.IsDNS1035Label(name)) != 0 ||
			len(validation.IsDNS1123Label(namespace)) != 0 {
			return dns.RcodeNameError, nil, nil
		}
		service := &corev1alpha1.Service{}
		if err := h.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, service); err != nil {
			if apierrors.IsNotFound(err) {
				return dns.RcodeNameError, nil, nil
			}
			return 0, nil, err
		}
		return dns.RcodeSuccess, h.serviceRecords(qname, service, qtype), nil
	}

	return dns.RcodeNameError, nil, nil
}

// namespaceExists reports whether any loom Service lives in namespace.
func (h *Handler) namespaceExists(ctx context.Context, namespace string) (bool, error) {
	services := &corev1alpha1.ServiceList{}
	if err := h.reader.List(ctx, services, client.InNamespace(namespace), client.Limit(1)); err != nil {
		return false, err
	}
	return len(services.Items) > 0, nil
}

// serviceRecords returns the A and/or AAAA records for the Service's cluster IPs that match qtype.
func (h *Handler) serviceRecords(qname string, service *corev1alpha1.Service, qtype uint16) []dns.RR {
	if qtype != dns.TypeA && qtype != dns.TypeAAAA && qtype != dns.TypeANY {
		return nil
	}
	var records []dns.RR
	for _, clusterIP := range service.Status.ClusterIPs {
		ip := net.ParseIP(clusterIP)
		if ip == nil {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			if qtype == dns.TypeA || qtype == dns.TypeANY {
				records = append(records, &dns.A{Hdr: h.header(qname, dns.TypeA), A: ip4})
			}
		} else if qtype == dns.TypeAAAA || qtype == dns.TypeANY {
			records = append(records, &dns.AAAA{Hdr: h.header(qname, dns.TypeAAAA), AAAA: ip})
		}
	}
	return records
}

func (h *Handler) soa() dns.RR {
	return &dns.SOA{
		Hdr:     h.header(h.zone, dns.TypeSOA),
		Ns:      h.nameserver(),
		Mbox:    "hostmaster." + h.zone,
		Serial:  h.serial,
		Refresh: 7200,
		Retry:   1800,
		Expire:  86400,
		Minttl:  h.ttl,
	}
}

func (h *Handler) ns() dns.RR {
	return &dns.NS{Hdr: h.header(h.zone, dns.TypeNS), Ns: h.nameserver()}
}

func (h *Handler) nameserver() string {
	return "ns.dns." + h.zone
}

func (h *Handler) header(name string, rrtype uint16) dns.RR_Header {
	return dns.RR_Header{Name: name, Rrtype: rrtype, Class: dns.ClassINET, Ttl: h.ttl}
}

func isUDP(w dns.ResponseWriter) bool {
	_, ok := w.RemoteAddr().(*net.UDPAddr)
	return ok
}
