package dns

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

// serveRecords starts a real DNS server on an ephemeral port backed by the
// given records and returns its address. Hermetic: no files, no fixed port.
func serveRecords(t *testing.T, domain string, records map[string]string) string {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}

	store := NewRecordStore(t.TempDir())
	store.mu.Lock()
	for host, ip := range records {
		store.merged[strings.ToLower(host)] = ip
	}
	store.mu.Unlock()

	server := &mdns.Server{
		PacketConn: pc,
		Handler:    &dnsHandler{domain: mdns.Fqdn(domain), store: store},
	}
	go func() { _ = server.ActivateAndServe() }()
	t.Cleanup(func() { _ = server.Shutdown() })

	return pc.LocalAddr().String()
}

func TestWaitForResolution_ReturnsWhenAllRecordsServed(t *testing.T) {
	want := map[string]string{
		"apim-api.gravitee.gck.local":     "172.18.0.5",
		"apim-console.gravitee.gck.local": "172.18.0.5",
	}
	addr := serveRecords(t, "gck.local", want)

	if err := WaitForResolution(context.Background(), addr, want, 5*time.Second); err != nil {
		t.Fatalf("expected records to be served: %v", err)
	}
}

// A wildcard record is never itself a queryable name — Lookup substitutes "*"
// for the first label — so the probe has to query through a synthetic label.
// Without that, *.kafka.gck.local could never be verified.
func TestWaitForResolution_WildcardProbedThroughSyntheticLabel(t *testing.T) {
	want := map[string]string{"*.kafka.gck.local": "172.18.0.7"}
	addr := serveRecords(t, "gck.local", want)

	if err := WaitForResolution(context.Background(), addr, want, 5*time.Second); err != nil {
		t.Fatalf("expected wildcard to be served: %v", err)
	}
}

func TestWaitForResolution_ErrorNamesUnresolvedHostnames(t *testing.T) {
	served := map[string]string{"a.gck.local": "10.0.0.1"}
	want := map[string]string{
		"a.gck.local": "10.0.0.1",
		"b.gck.local": "10.0.0.2",
	}
	addr := serveRecords(t, "gck.local", served)

	err := WaitForResolution(context.Background(), addr, want, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error when a record is not served")
	}
	if !strings.Contains(err.Error(), "b.gck.local") {
		t.Fatalf("error should name the unresolved hostname, got: %v", err)
	}
	if strings.Contains(err.Error(), "a.gck.local") {
		t.Fatalf("error should not name the resolved hostname, got: %v", err)
	}
}

// Guards the squatter case: another process holding the port answers, but with
// the wrong address. The SIGKILL that clears a stale gck server cannot touch a
// server owned by another user, so "something answered" is not readiness.
func TestWaitForResolution_WrongIPIsNotReady(t *testing.T) {
	addr := serveRecords(t, "gck.local", map[string]string{"a.gck.local": "10.9.9.9"})
	want := map[string]string{"a.gck.local": "10.0.0.1"}

	if err := WaitForResolution(context.Background(), addr, want, 500*time.Millisecond); err == nil {
		t.Fatal("expected an error when the served IP differs from the expected one")
	}
}

// The fork-to-bind window: startDNSServer returns as soon as fork/exec
// succeeds, while the child still has to reach ListenAndServe.
func TestWaitForResolution_NoServerListening(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close() // nothing is bound here now

	want := map[string]string{"a.gck.local": "10.0.0.1"}
	if err := WaitForResolution(context.Background(), addr, want, 500*time.Millisecond); err == nil {
		t.Fatal("expected an error when no server is listening")
	}
}

// A server with no records is still ready — it is bound and answering. This is
// the check for contexts that enable DNS but declare nothing to serve.
func TestWaitForServer_NXDOMAINCountsAsReady(t *testing.T) {
	addr := serveRecords(t, "gck.local", nil)

	if err := WaitForServer(context.Background(), addr, "gck.local", 5*time.Second); err != nil {
		t.Fatalf("a bound server answering NXDOMAIN is ready: %v", err)
	}
}

func TestWaitForResolution_EmptyWantIsImmediatelyReady(t *testing.T) {
	if err := WaitForResolution(context.Background(), "127.0.0.1:1", nil, time.Millisecond); err != nil {
		t.Fatalf("nothing to wait for: %v", err)
	}
}

// Without an SOA the caching resolver picks its own negative TTL, so a
// NXDOMAIN served during startup can be replayed long after records land.
func TestHandleLocal_NXDOMAINCarriesSOA(t *testing.T) {
	addr := serveRecords(t, "gck.local", nil)

	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn("missing.gck.local"), mdns.TypeA)
	client := &mdns.Client{Net: "udp", Timeout: 2 * time.Second}
	resp, _, err := client.Exchange(msg, addr)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if resp.Rcode != mdns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN, got %s", mdns.RcodeToString[resp.Rcode])
	}
	if len(resp.Ns) != 1 {
		t.Fatalf("expected exactly one authority record, got %d", len(resp.Ns))
	}
	soa, ok := resp.Ns[0].(*mdns.SOA)
	if !ok {
		t.Fatalf("expected an SOA in the authority section, got %T", resp.Ns[0])
	}
	if soa.Minttl > 5 {
		t.Fatalf("negative TTL should be short, got %d", soa.Minttl)
	}
}
