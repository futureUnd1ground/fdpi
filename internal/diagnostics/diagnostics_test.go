package diagnostics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeStagesAndTLSValidation(t *testing.T) {
	ctx := context.Background()
	lookup := func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	failDial := func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("dial failure") }
	r := probe(ctx, "example.com", func(context.Context, string) ([]string, error) { return nil, errors.New("DNS failure") }, failDial, &tls.Config{})
	if r.Stage != "dns" || r.Reachable {
		t.Fatal(r)
	}
	r = probe(ctx, "example.com", lookup, failDial, &tls.Config{})
	if r.Stage != "tcp" || r.Reachable {
		t.Fatal(r)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(451) }))
	defer server.Close()
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	r = probe(ctx, "example.com", lookup, dial, &tls.Config{MinVersion: tls.VersionTLS12})
	if r.Stage != "tls" || r.Reachable {
		t.Fatalf("untrusted certificate accepted: %+v", r)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	r = probe(ctx, "example.com", lookup, dial, &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots})
	if !r.Reachable || r.HTTP != 451 || r.Stage != "http-response" {
		t.Fatalf("HTTP application failure misclassified: %+v", r)
	}
}
