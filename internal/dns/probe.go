package dns

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	mdns "github.com/miekg/dns"
	"k8s.io/klog/v2"
)

const (
	// probeLabel stands in for the first label of a wildcard record. Lookup
	// (records.go) replaces the first label with "*" before matching, so a
	// wildcard like "*.kafka.gck.local" is never itself a queryable name —
	// something has to occupy that position. Fixed rather than random so the
	// log line is stable between runs.
	probeLabel = "gck-probe"

	// Between rounds, not a ticker: with a 2s exchange timeout a ticker this
	// short would let exchanges overlap.
	probeInterval = 250 * time.Millisecond
	probeExchange = 2 * time.Second
)

// WaitForServer blocks until the DNS server at addr answers a query for the
// domain at all, or the timeout expires.
//
// Any response proves the socket is bound, NXDOMAIN included — this is the
// readiness check for a server with no records to serve. Only transport
// errors count as not-ready.
func WaitForServer(ctx context.Context, addr, domain string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	name := probeLabel + "." + strings.TrimPrefix(domain, ".")

	var lastErr error
	for {
		_, _, err := exchangeA(ctx, addr, name)
		if err == nil {
			return nil
		}
		lastErr = err

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"DNS server at %s did not answer within %s (last error: %v); "+
					"it may have failed to bind — check for another process on that port",
				addr, timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(probeInterval):
		}
	}
}

// WaitForResolution blocks until the DNS server at addr answers an A query for
// every hostname in want with that hostname's expected IP, or the timeout
// expires.
//
// This is the check gck never had: the server was spawned and declared started
// on the strength of fork/exec returning, while the child still had to reach
// ListenAndServe, and nothing ever asked it a question. Matching the IP rather
// than merely "an answer" is free here and catches a foreign process squatting
// on the port — reachable in practice, because the SIGKILL that clears a stale
// server cannot touch one owned by another user.
func WaitForResolution(ctx context.Context, addr string, want map[string]string, timeout time.Duration) error {
	if len(want) == 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	pending := make(map[string]string, len(want))
	for host, ip := range want {
		pending[host] = ip
	}

	for {
		for _, host := range sortedKeys(pending) {
			ips, rcode, err := exchangeA(ctx, addr, probeName(host))
			if err != nil {
				klog.V(2).Infof("DNS probe %s: %v", host, err)
				continue
			}
			if rcode != mdns.RcodeSuccess {
				klog.V(2).Infof("DNS probe %s: %s", host, mdns.RcodeToString[rcode])
				continue
			}
			for _, got := range ips {
				if got == pending[host] {
					delete(pending, host)
					break
				}
			}
		}

		if len(pending) == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"DNS server at %s did not serve %d/%d record(s) within %s (%s); "+
					"the server may have failed to bind — check for another process on that port",
				addr, len(pending), len(want), timeout, strings.Join(sortedKeys(pending), ", "))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(probeInterval):
		}
	}
}

// probeName makes a wildcard record queryable by substituting a real label for
// the "*". Non-wildcard hostnames are returned unchanged.
func probeName(hostname string) string {
	if rest, ok := strings.CutPrefix(hostname, "*."); ok {
		return probeLabel + "." + rest
	}
	return hostname
}

func exchangeA(ctx context.Context, addr, name string) ([]string, int, error) {
	client := &mdns.Client{Net: "udp", Timeout: probeExchange}
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(name), mdns.TypeA)

	resp, _, err := client.ExchangeContext(ctx, msg, addr)
	if err != nil {
		return nil, 0, err
	}

	var ips []string
	for _, rr := range resp.Answer {
		if a, ok := rr.(*mdns.A); ok {
			ips = append(ips, a.A.String())
		}
	}
	return ips, resp.Rcode, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
