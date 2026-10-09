package zeroconf

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestHandleQuestionAnswersHostnameA(t *testing.T) {
	svc := NewServiceEntry("alias", "_kportal._tcp", "local.")
	svc.HostName = "alias.local."
	svc.AddrIPv4 = []net.IP{net.IPv4(127, 0, 0, 1)}
	s := &Server{service: svc, ttl: 3200}

	q := dns.Question{Name: "alias.local.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
	resp := new(dns.Msg)
	if err := s.handleQuestion(q, resp, new(dns.Msg), 0); err != nil {
		t.Fatal(err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("want 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || !a.A.Equal(net.IPv4(127, 0, 0, 1)) || a.Hdr.Name != "alias.local." {
		t.Fatalf("unexpected answer: %v", resp.Answer[0])
	}
}

func TestHandleQuestionIgnoresOtherTypesForHostname(t *testing.T) {
	svc := NewServiceEntry("alias", "_kportal._tcp", "local.")
	svc.HostName = "alias.local."
	svc.AddrIPv4 = []net.IP{net.IPv4(127, 0, 0, 1)}
	s := &Server{service: svc, ttl: 3200}

	q := dns.Question{Name: "alias.local.", Qtype: dns.TypeTXT, Qclass: dns.ClassINET}
	resp := new(dns.Msg)
	_ = s.handleQuestion(q, resp, new(dns.Msg), 0)
	if len(resp.Answer) != 0 {
		t.Fatalf("want no answer, got %d", len(resp.Answer))
	}
}
