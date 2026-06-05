// Package dns implements a local DNS server that resolves hostnames from
// per-cluster record files stored in .gck/dns/. The server watches the
// directory for changes, hot-reloads records, and shuts itself down
// automatically when all record files are removed.
package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	mdns "github.com/miekg/dns"
	"k8s.io/klog/v2"
)

// Config holds settings for the local DNS server.
type Config struct {
	// Dir is the path to the record file directory (e.g. ~/.gck/dns/).
	Dir string
	// Domain is the DNS domain to serve (e.g. "gck.local").
	Domain string
	// Addr is the UDP listen address (e.g. "127.0.0.1:5353").
	Addr string
	// Upstream is the address of the DNS server to forward non-matching
	// queries to (e.g. "8.8.8.8:53"). Empty disables forwarding.
	Upstream string
}

// Run starts the DNS server and blocks until ctx is cancelled or all record
// files are removed (auto-shutdown). It loads existing record files, starts
// the file watcher, and serves DNS queries.
func Run(ctx context.Context, cfg Config) error {
	store := NewRecordStore(cfg.Dir)
	if err := store.Load(); err != nil {
		return fmt.Errorf("loading DNS records: %w", err)
	}

	select {
	case <-store.Empty():
		klog.Info("no record files found, exiting")
		return nil
	default:
	}

	klog.Infof("loaded %d DNS record(s)", store.RecordCount())

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := store.Watch(ctx); err != nil && ctx.Err() == nil {
			klog.Errorf("record watcher failed: %v", err)
			cancel()
		}
	}()

	go func() {
		select {
		case <-store.Empty():
			klog.Info("all record files removed, shutting down")
			cancel()
		case <-ctx.Done():
		}
	}()

	h := &dnsHandler{
		domain:   mdns.Fqdn(cfg.Domain),
		store:    store,
		upstream: cfg.Upstream,
	}

	server := &mdns.Server{
		Addr:    cfg.Addr,
		Net:     "udp",
		Handler: h,
	}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown()
	}()

	klog.Infof("DNS server listening on %s (domain: %s)", cfg.Addr, cfg.Domain)
	if err := server.ListenAndServe(); err != nil {
		select {
		case <-ctx.Done():
			return nil
		default:
			return fmt.Errorf("DNS server failed: %w", err)
		}
	}

	return nil
}

type dnsHandler struct {
	domain   string // FQDN with trailing dot
	store    *RecordStore
	upstream string
}

func (h *dnsHandler) ServeDNS(w mdns.ResponseWriter, r *mdns.Msg) {
	if len(r.Question) == 0 {
		return
	}

	q := r.Question[0]
	qname := strings.ToLower(q.Name)

	if mdns.IsSubDomain(h.domain, qname) {
		if q.Qtype == mdns.TypeA {
			h.handleLocal(w, r, qname)
			return
		}
		// Return immediate empty response for non-A queries (e.g. AAAA)
		// on local hostnames to avoid forwarding to upstream and timing out.
		msg := new(mdns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true
		_ = w.WriteMsg(msg)
		return
	}

	h.forward(w, r)
}

func (h *dnsHandler) handleLocal(w mdns.ResponseWriter, r *mdns.Msg, qname string) {
	hostname := strings.TrimSuffix(qname, ".")

	ip, ok := h.store.Lookup(hostname)
	if !ok {
		msg := new(mdns.Msg)
		msg.SetRcode(r, mdns.RcodeNameError)
		msg.Authoritative = true
		_ = w.WriteMsg(msg)
		return
	}

	msg := new(mdns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	msg.Answer = append(msg.Answer, &mdns.A{
		Hdr: mdns.RR_Header{
			Name:   qname,
			Rrtype: mdns.TypeA,
			Class:  mdns.ClassINET,
			Ttl:    5,
		},
		A: net.ParseIP(ip),
	})
	_ = w.WriteMsg(msg)
}

func (h *dnsHandler) forward(w mdns.ResponseWriter, r *mdns.Msg) {
	if h.upstream == "" {
		msg := new(mdns.Msg)
		msg.SetRcode(r, mdns.RcodeServerFailure)
		_ = w.WriteMsg(msg)
		return
	}

	client := &mdns.Client{
		Net:     "udp",
		Timeout: 5 * time.Second,
	}
	resp, _, err := client.Exchange(r, h.upstream)
	if err != nil {
		klog.V(2).Infof("upstream DNS query failed: %v", err)
		msg := new(mdns.Msg)
		msg.SetRcode(r, mdns.RcodeServerFailure)
		_ = w.WriteMsg(msg)
		return
	}
	_ = w.WriteMsg(resp)
}
