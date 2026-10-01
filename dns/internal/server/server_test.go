package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/miekg/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

const testZone = "loom.local."

func newService(namespace, name string, clusterIPs ...string) *corev1alpha1.Service {
	return &corev1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Status:     corev1alpha1.ServiceStatus{ClusterIPs: clusterIPs},
	}
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		newService("default", "web-server", "10.96.0.10", "fd00:10:96::a"),
		newService("default", "pending"),
		newService("other", "v4only", "10.96.0.20"),
	).Build()
	return New(reader, testZone, 5, logr.Discard())
}

// recorder is a dns.ResponseWriter that keeps the written message.
type recorder struct {
	msg *dns.Msg
}

func (r *recorder) LocalAddr() net.Addr       { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53} }
func (r *recorder) RemoteAddr() net.Addr      { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40000} }
func (r *recorder) WriteMsg(m *dns.Msg) error { r.msg = m; return nil }
func (r *recorder) Write([]byte) (int, error) { return 0, errors.New("not supported") }
func (r *recorder) Close() error              { return nil }
func (r *recorder) TsigStatus() error         { return nil }
func (r *recorder) TsigTimersOnly(bool)       {}
func (r *recorder) Hijack()                   {}

func query(t *testing.T, h *Handler, req *dns.Msg) *dns.Msg {
	t.Helper()
	w := &recorder{}
	h.ServeDNS(w, req)
	if w.msg == nil {
		t.Fatalf("no response written")
	}
	return w.msg
}

// rdata returns the A/AAAA addresses and the types of the other records in rrs.
func rdata(rrs []dns.RR) []string {
	var out []string
	for _, rr := range rrs {
		switch rr := rr.(type) {
		case *dns.A:
			out = append(out, rr.A.String())
		case *dns.AAAA:
			out = append(out, rr.AAAA.String())
		default:
			out = append(out, dns.TypeToString[rr.Header().Rrtype])
		}
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestServeDNS(t *testing.T) {
	h := newTestHandler(t)

	tests := []struct {
		name          string
		qname         string
		qtype         uint16
		rcode         int
		authoritative bool
		answer        []string
		soaInNs       bool
	}{
		{"A record", "web-server.default.svc.loom.local.", dns.TypeA,
			dns.RcodeSuccess, true, []string{"10.96.0.10"}, false},
		{"AAAA record", "web-server.default.svc.loom.local.", dns.TypeAAAA,
			dns.RcodeSuccess, true, []string{"fd00:10:96::a"}, false},
		{"ANY returns both families", "web-server.default.svc.loom.local.", dns.TypeANY,
			dns.RcodeSuccess, true, []string{"10.96.0.10", "fd00:10:96::a"}, false},
		{"mixed case name", "Web-Server.DEFAULT.Svc.Loom.Local.", dns.TypeA,
			dns.RcodeSuccess, true, []string{"10.96.0.10"}, false},
		{"unsupported type on existing Service is NODATA", "web-server.default.svc.loom.local.", dns.TypeMX,
			dns.RcodeSuccess, true, nil, true},
		{"missing family is NODATA", "v4only.other.svc.loom.local.", dns.TypeAAAA,
			dns.RcodeSuccess, true, nil, true},
		{"Service without IPs is NODATA", "pending.default.svc.loom.local.", dns.TypeA,
			dns.RcodeSuccess, true, nil, true},
		{"missing Service is NXDOMAIN", "nope.default.svc.loom.local.", dns.TypeA,
			dns.RcodeNameError, true, nil, true},
		{"Service in another namespace is NXDOMAIN", "web-server.other.svc.loom.local.", dns.TypeA,
			dns.RcodeNameError, true, nil, true},
		{"namespace with Services is NODATA", "default.svc.loom.local.", dns.TypeA,
			dns.RcodeSuccess, true, nil, true},
		{"namespace without Services is NXDOMAIN", "empty.svc.loom.local.", dns.TypeA,
			dns.RcodeNameError, true, nil, true},
		{"svc label is NODATA", "svc.loom.local.", dns.TypeA,
			dns.RcodeSuccess, true, nil, true},
		{"unknown label below zone is NXDOMAIN", "pod.loom.local.", dns.TypeA,
			dns.RcodeNameError, true, nil, true},
		{"invalid Service name is NXDOMAIN", "1web.default.svc.loom.local.", dns.TypeA,
			dns.RcodeNameError, true, nil, true},
		{"deeper name is NXDOMAIN", "a.web-server.default.svc.loom.local.", dns.TypeA,
			dns.RcodeNameError, true, nil, true},
		{"apex SOA", "loom.local.", dns.TypeSOA,
			dns.RcodeSuccess, true, []string{"SOA"}, false},
		{"apex NS", "loom.local.", dns.TypeNS,
			dns.RcodeSuccess, true, []string{"NS"}, false},
		{"apex A is NODATA", "loom.local.", dns.TypeA,
			dns.RcodeSuccess, true, nil, true},
		{"out of zone is REFUSED", "web-server.default.svc.cluster.local.", dns.TypeA,
			dns.RcodeRefused, false, nil, false},
		{"zone suffix without label boundary is REFUSED", "xloom.local.", dns.TypeA,
			dns.RcodeRefused, false, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(tt.qname, tt.qtype)
			resp := query(t, h, req)

			if resp.Rcode != tt.rcode {
				t.Errorf("unexpected rcode: got %s, want %s", dns.RcodeToString[resp.Rcode], dns.RcodeToString[tt.rcode])
			}
			if resp.Authoritative != tt.authoritative {
				t.Errorf("unexpected AA flag: got %v, want %v", resp.Authoritative, tt.authoritative)
			}
			if got := rdata(resp.Answer); !equal(got, tt.answer) {
				t.Errorf("unexpected answer: got %v, want %v", got, tt.answer)
			}
			if got := rdata(resp.Ns); tt.soaInNs != equal(got, []string{"SOA"}) {
				t.Errorf("unexpected authority section: %v", got)
			}
			for _, rr := range resp.Answer {
				if rr.Header().Ttl != 5 {
					t.Errorf("unexpected TTL: %d", rr.Header().Ttl)
				}
				if rr.Header().Rrtype == dns.TypeA && rr.Header().Name != dns.CanonicalName(tt.qname) {
					t.Errorf("unexpected owner name: %s", rr.Header().Name)
				}
			}
		})
	}
}

func TestServeDNS_InvalidRequests(t *testing.T) {
	h := newTestHandler(t)

	notify := new(dns.Msg)
	notify.SetNotify(testZone)
	if resp := query(t, h, notify); resp.Rcode != dns.RcodeNotImplemented {
		t.Errorf("unexpected rcode for NOTIFY: %s", dns.RcodeToString[resp.Rcode])
	}

	noQuestion := new(dns.Msg)
	noQuestion.Id = dns.Id()
	if resp := query(t, h, noQuestion); resp.Rcode != dns.RcodeFormatError {
		t.Errorf("unexpected rcode for empty question: %s", dns.RcodeToString[resp.Rcode])
	}

	chaos := new(dns.Msg)
	chaos.SetQuestion("web-server.default.svc.loom.local.", dns.TypeA)
	chaos.Question[0].Qclass = dns.ClassCHAOS
	if resp := query(t, h, chaos); resp.Rcode != dns.RcodeRefused {
		t.Errorf("unexpected rcode for CHAOS class: %s", dns.RcodeToString[resp.Rcode])
	}
}

// failingReader is a client.Reader whose reads always fail.
type failingReader struct{}

func (failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("cache unavailable")
}

func (failingReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("cache unavailable")
}

func TestServeDNS_ReaderErrorIsServfail(t *testing.T) {
	h := New(failingReader{}, testZone, 5, logr.Discard())

	for _, qname := range []string{"web-server.default.svc.loom.local.", "default.svc.loom.local."} {
		req := new(dns.Msg)
		req.SetQuestion(qname, dns.TypeA)
		resp := query(t, h, req)
		if resp.Rcode != dns.RcodeServerFailure {
			t.Errorf("%s: unexpected rcode: %s", qname, dns.RcodeToString[resp.Rcode])
		}
		if resp.Authoritative {
			t.Errorf("%s: SERVFAIL should not be authoritative", qname)
		}
	}
}

func TestServeDNS_OverUDP(t *testing.T) {
	h := newTestHandler(t)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	started := make(chan struct{})
	srv := &dns.Server{PacketConn: conn, Handler: h, NotifyStartedFunc: func() { close(started) }}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	<-started

	req := new(dns.Msg)
	req.SetQuestion("web-server.default.svc.loom.local.", dns.TypeA)
	req.SetEdns0(4096, false)

	c := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	resp, _, err := c.Exchange(req, conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess || !resp.Authoritative {
		t.Errorf("unexpected response header: rcode=%s aa=%v", dns.RcodeToString[resp.Rcode], resp.Authoritative)
	}
	if got := rdata(resp.Answer); !equal(got, []string{"10.96.0.10"}) {
		t.Errorf("unexpected answer: %v", got)
	}
	opt := resp.IsEdns0()
	if opt == nil || opt.UDPSize() != udpBufferSize {
		t.Errorf("unexpected EDNS0 record: %v", opt)
	}
}
