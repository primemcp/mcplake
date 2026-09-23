package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// distDir writes a minimal built SPA: the shell plus one hashed asset, the
// shape `bun run build` actually produces.
func distDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>shell</title>"), 0o644))
	require(t, os.MkdirAll(filepath.Join(root, "assets"), 0o755))
	require(t, os.WriteFile(filepath.Join(root, "assets", "index-abc123.js"), []byte("console.log(1)"), 0o644))
	return root
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func newTestHandler(t *testing.T, cfg Config) http.Handler {
	t.Helper()
	if cfg.Root == "" {
		cfg.Root = distDir(t)
	}
	if cfg.APIPrefix == "" {
		cfg.APIPrefix = defaultAPIPrefix
	}
	h, err := newHandler(cfg)
	require(t, err)
	return h
}

func TestServesTheBuiltAssets(t *testing.T) {
	h := newTestHandler(t, Config{})

	rec := get(t, h, "/assets/index-abc123.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Fatalf("got %q", rec.Body.String())
	}
}

// The frontend has no server-side router, so every route it owns has to
// arrive as index.html and be resolved in the browser. /mcps/postgres-ro is a
// real deep link the admin UI generates.
func TestUnknownRouteServesTheAppShell(t *testing.T) {
	h := newTestHandler(t, Config{})

	for _, target := range []string{"/", "/mcps", "/mcps/postgres-ro", "/users"} {
		rec := get(t, h, target)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", target, rec.Code)
		}
		if rec.Body.String() != "<!doctype html><title>shell</title>" {
			t.Errorf("%s: got %q, want the shell", target, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control %q — the shell names this build's hashed assets "+
				"and must not be served from a previous one", target, got)
		}
	}
}

// A missing asset must 404 rather than fall back. Returning the shell for a
// .js request hands the browser HTML with a JavaScript content type, which
// surfaces as a syntax error instead of a clear miss — the usual symptom of a
// stale or half-copied build.
func TestMissingAssetIsNotFoundRatherThanTheShell(t *testing.T) {
	h := newTestHandler(t, Config{})

	rec := get(t, h, "/assets/index-stale999.js")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404; body %q", rec.Code, rec.Body.String())
	}
}

func TestDoesNotServeFilesOutsideTheAssetRoot(t *testing.T) {
	root := distDir(t)
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	require(t, os.WriteFile(secret, []byte("do not serve me"), 0o600))
	h := newTestHandler(t, Config{Root: root})

	for _, target := range []string{
		"/../secret.txt",
		"/%2e%2e/secret.txt",
		"/assets/../../secret.txt",
	} {
		rec := get(t, h, target)
		if rec.Body.String() == "do not serve me" {
			t.Fatalf("%s: served a file from outside the root", target)
		}
	}
}

func TestProxiesTheAPIPrefixToTheGateway(t *testing.T) {
	var gotPath, gotHeader string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"postgres-ro"}]`))
	}))
	defer gateway.Close()

	h := newTestHandler(t, Config{APITarget: gateway.URL})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/mcps", nil)
	req.Header.Set("Authorization", "Bearer token")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if gotPath != "/admin/mcps" {
		t.Errorf("gateway saw %q, want /admin/mcps — the prefix is the gateway's own, not stripped", gotPath)
	}
	// Without this the admin API sees every request unauthenticated and the
	// whole UI 401s, which is the first thing a broken proxy looks like.
	if gotHeader != "Bearer token" {
		t.Errorf("Authorization not forwarded, got %q", gotHeader)
	}
	if rec.Body.String() != `[{"name":"postgres-ro"}]` {
		t.Errorf("got %q", rec.Body.String())
	}
}

// The bare prefix (no trailing path) is the one an operator hits by hand and
// the one a naive mux misses.
func TestProxiesTheBarePrefix(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("from gateway"))
	}))
	defer gateway.Close()

	h := newTestHandler(t, Config{APITarget: gateway.URL})

	if got := get(t, h, "/admin").Body.String(); got != "from gateway" {
		t.Fatalf("got %q, want the gateway's response", got)
	}
}

// A path that merely starts with the prefix's letters is not under it.
// "/administrators" must reach the SPA, not the admin API.
func TestPrefixMatchIsPathSegmented(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("from gateway"))
	}))
	defer gateway.Close()

	h := newTestHandler(t, Config{APITarget: gateway.URL})

	if got := get(t, h, "/administrators").Body.String(); got == "from gateway" {
		t.Fatal("/administrators was proxied; only /admin and /admin/... belong to the API")
	}
}

func TestPrefixIsNormalized(t *testing.T) {
	for _, prefix := range []string{"/admin", "admin", "/admin/"} {
		gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("from gateway"))
		}))
		h := newTestHandler(t, Config{APITarget: gateway.URL, APIPrefix: prefix})

		if got := get(t, h, "/admin/mcps").Body.String(); got != "from gateway" {
			t.Errorf("prefix %q: got %q, want the gateway's response", prefix, got)
		}
		gateway.Close()
	}
}

// With no target configured nothing is forwarded, and the prefix must not
// silently become part of the SPA's routes either — it falls through to the
// shell, which is what a deployment fronted by its own ingress expects.
func TestNoAPITargetDisablesProxying(t *testing.T) {
	h := newTestHandler(t, Config{})

	rec := get(t, h, "/admin/mcps")

	if rec.Code != http.StatusOK || rec.Body.String() != "<!doctype html><title>shell</title>" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

// A gateway that is down must produce the admin API's own {error,message}
// shape, so the UI's existing error path renders something truthful instead
// of a blank 502.
func TestUnreachableGatewayReturnsAStructuredError(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target := gateway.URL
	gateway.Close() // nothing is listening now

	h := newTestHandler(t, Config{APITarget: target})

	rec := get(t, h, "/admin/mcps")

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "gateway_unreachable") {
		t.Errorf("got %q, want a gateway_unreachable error", body)
	}
}

func TestHealthz(t *testing.T) {
	h := newTestHandler(t, Config{})

	rec := get(t, h, "/healthz")

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

// A missing or mis-mounted build must fail at startup with a message naming
// the path, not serve 404s to a browser that then shows a blank page.
func TestRefusesToStartWithoutABuiltSPA(t *testing.T) {
	if _, err := newHandler(Config{Root: t.TempDir(), APIPrefix: "/admin"}); err == nil {
		t.Fatal("expected an error for a root with no index.html")
	}
	if _, err := newHandler(Config{Root: "", APIPrefix: "/admin"}); err == nil {
		t.Fatal("expected an error for an empty root")
	}
}

func TestRejectsAnUnusableAPITarget(t *testing.T) {
	if _, err := newHandler(Config{Root: distDir(t), APIPrefix: "/admin", APITarget: "localhost:9091"}); err == nil {
		t.Fatal("expected an error for a target with no scheme")
	}
}
