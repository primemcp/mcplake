package internal_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/mcp"
	"github.com/atsokha/mcplake/router"
	"github.com/golang-jwt/jwt/v5"
)

// BenchmarkToolCall exercises the fully-wired POST /v1/call handler with
// *real* JWT validation (signature check against a real RSA key via a local
// JWKS server) and *real* policy evaluation (JSONPath+regexp ClaimRule
// matching through router.Engine) — only the downstream MCP call is a
// no-op mock, per this ticket's scope, so the measurement isolates gateway
// overhead (auth + policy + HTTP/JSON plumbing) from real downstream tool
// latency, while still capturing the two most expensive real components:
// RSA signature verification and claim-rule evaluation.
//
// This validates the ~1-2ms gateway-overhead target from README.md /
// ADR-0001.
//
// Run with:
//
//	go test ./gateway/... -bench=BenchmarkToolCall -benchmem -run=^$
//
// Alongside the standard ns/op and allocs/op that -benchmem reports (which,
// under b.RunParallel, reflect wall-time-per-iteration across all parallel
// workers combined — a throughput figure, not per-request latency), this
// also reports p50/p99 per-request latency (in microseconds) as custom
// metrics, computed from wall-clock time recorded around each individual
// request. Results as of this ticket are recorded in ADR-0001's Validation
// section.
func BenchmarkToolCall(b *testing.B) {
	validator, token := benchmarkAuthenticator(b)
	policy := benchmarkPolicyEngine(b)

	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			raw := json.RawMessage(`{"id":42,"email":"a@example.com","hashed_password":"secret"}`)
			return &mcp.ToolResponse{Raw: raw}, nil
		},
	})

	addr, cleanup := startTestGatewayWithConfig(b, internal.Config{
		Authenticator: validator,
		Policy:        policy,
		Resolver:      resolver,
	})
	defer cleanup()

	url := "http://" + addr + "/v1/call"
	const requestBody = `{"mcp":"postgres-ro","tool":"get_user","arguments":{"id":42}}`
	authHeader := "Bearer " + token

	// A connection pool sized to comfortably cover GOMAXPROCS parallel
	// callers, so the benchmark measures gateway + loopback overhead, not
	// artificial client-side connection contention.
	transport := &http.Transport{MaxConnsPerHost: 64}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	var mu sync.Mutex
	latencies := make([]time.Duration, 0, b.N)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			start := time.Now()

			req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
			if err != nil {
				b.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authHeader)

			resp, err := client.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			elapsed := time.Since(start)

			if resp.StatusCode != http.StatusOK {
				b.Fatalf("unexpected status %d", resp.StatusCode)
			}

			mu.Lock()
			latencies = append(latencies, elapsed)
			mu.Unlock()
		}
	})

	b.StopTimer()

	p50, p99 := percentiles(latencies)
	b.ReportMetric(float64(p50.Microseconds()), "p50-µs/op")
	b.ReportMetric(float64(p99.Microseconds()), "p99-µs/op")
}

// benchmarkAuthenticator builds a real *auth.Validator backed by a local
// JWKS server and returns it alongside a real, signed JWT it will accept -
// so the benchmark pays the actual cost of RSA signature verification, not
// a fake stand-in for it.
func benchmarkAuthenticator(b *testing.B) (*auth.Validator, string) {
	b.Helper()

	const (
		issuer   = "https://issuer.bench.test"
		audience = "mcp-gateway-bench"
		kid      = "bench-key"
	)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}

	jwk, err := jwkset.NewJWKFromKey(&key.PublicKey, jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{KID: kid, ALG: jwkset.AlgRS256},
	})
	if err != nil {
		b.Fatal(err)
	}
	jwksBody, err := json.Marshal(jwkset.JWKSMarshal{Keys: []jwkset.JWKMarshal{jwk.Marshal()}})
	if err != nil {
		b.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))
	b.Cleanup(srv.Close)

	validator, err := auth.NewValidator(context.Background(), auth.Config{
		JWKSURL:  srv.URL,
		Issuer:   issuer,
		Audience: audience,
	})
	if err != nil {
		b.Fatal(err)
	}

	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":  issuer,
		"aud":  audience,
		"sub":  "bench-user",
		"role": "user",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		b.Fatal(err)
	}

	return validator, signed
}

// benchmarkPolicyEngine builds a real *router.Engine with one AccessPolicy
// granting postgres-ro/get_user to role=user, and one FilterPolicy dropping
// hashed_password for the same audience — real JSONPath+regexp evaluation,
// not a fake.
func benchmarkPolicyEngine(b *testing.B) *router.Engine {
	b.Helper()

	rule, err := router.NewClaimRule("$.role", "^user$")
	if err != nil {
		b.Fatal(err)
	}
	matcher := router.ClaimMatcher{Rules: []router.ClaimRule{rule}}

	return router.NewEngine(
		[]router.AccessPolicy{
			{
				Name:  "bench-access",
				Match: matcher,
				Grants: []router.Grant{
					{MCP: "postgres-ro", Tools: []string{"get_user"}},
				},
			},
		},
		[]router.FilterPolicy{
			{
				Name:       "bench-filter",
				Match:      matcher,
				MCP:        "postgres-ro",
				Tool:       "get_user",
				DropFields: []string{"$.hashed_password"},
			},
		},
	)
}

// percentiles returns the p50 and p99 of durations. durations is sorted in
// place.
func percentiles(durations []time.Duration) (p50, p99 time.Duration) {
	if len(durations) == 0 {
		return 0, 0
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	idx := func(pct int) int {
		i := len(durations) * pct / 100
		if i >= len(durations) {
			i = len(durations) - 1
		}
		return i
	}
	return durations[idx(50)], durations[idx(99)]
}

// TestPercentiles_Sanity is a quick unit check on the percentile helper
// itself, independent of running the full benchmark.
func TestPercentiles_Sanity(t *testing.T) {
	durations := make([]time.Duration, 100)
	for i := range durations {
		durations[i] = time.Duration(i+1) * time.Millisecond // 1ms..100ms
	}

	p50, p99 := percentiles(durations)

	if p50 != 50*time.Millisecond && p50 != 51*time.Millisecond {
		t.Fatalf("unexpected p50: %v", p50)
	}
	if p99 != 99*time.Millisecond && p99 != 100*time.Millisecond {
		t.Fatalf("unexpected p99: %v", p99)
	}
}
