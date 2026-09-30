// Package main serves the mcplake admin SPA from its own container and
// forwards the admin API to the gateway, so a browser sees one origin.
//
// See docs/architecture/decisions/0020-serve-the-admin-ui-from-its-own-container.rst.
package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
)

// Config is everything this server needs to run.
type Config struct {
	// Addr is the listen address.
	Addr string
	// Root is the directory holding the built SPA (index.html and assets).
	Root string
	// APIPrefix is the single path prefix forwarded to APITarget. Every
	// other path is served from Root.
	APIPrefix string
	// APITarget is the gateway control plane's base URL. Empty disables
	// proxying entirely, which is what a deployment fronted by its own
	// ingress wants -- see newHandler.
	APITarget string
}

// newHandler builds the whole routing surface: the API prefix proxied to the
// gateway, everything else served from the built SPA.
//
// The order matters and is the point of this program. The UI fetches the
// admin API with *relative* URLs, and ADR-0014's PKCE redirect URI names the
// origin the operator browsed to, so the assets and the API have to answer on
// one origin. Serving them from two would mean a CORS allowlist on the admin
// API -- the most sensitive surface in the system -- plus a configurable API
// base URL in the bundle and a second registered redirect URI. Forwarding one
// prefix here costs a few lines instead.
func newHandler(cfg Config) (http.Handler, error) {
	assets, err := newAssetHandler(cfg.Root)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	if cfg.APITarget != "" {
		proxy, err := newAPIProxy(cfg.APITarget)
		if err != nil {
			return nil, err
		}
		// Only this prefix is forwarded. An open proxy in front of an admin
		// API would be a much bigger hole than the one this closes, so the
		// forwarded set is a single configured prefix and never a wildcard
		// derived from the request.
		prefix := normalizePrefix(cfg.APIPrefix)
		mux.Handle(prefix, proxy)
		mux.Handle(prefix+"/", proxy)
	}

	mux.Handle("/", assets)
	return mux, nil
}

// normalizePrefix coerces a prefix to a leading slash and no trailing one, so
// "/admin", "admin" and "/admin/" all register the same routes.
func normalizePrefix(prefix string) string {
	prefix = "/" + strings.Trim(prefix, "/")
	if prefix == "/" {
		// Forwarding everything would leave nothing to serve and turn this
		// into a bare proxy; treat it as a configuration error upstream.
		return "/__unset"
	}
	return prefix
}

// newAPIProxy forwards to the gateway's control plane.
func newAPIProxy(target string) (http.Handler, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("webui: parse api target %q: %w", target, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("webui: api target must be an absolute URL (got %q)", target)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	// The default ErrorHandler logs to stderr and writes a bare 502 with an
	// empty body; the UI shows a generic failure for that. A JSON body in the
	// admin API's own {error,message} shape means the existing ErrorNotice
	// path renders something truthful when the gateway is down.
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error":"gateway_unreachable","message":%q}`, err.Error())
	}
	return proxy, nil
}

// assetHandler serves the built SPA with history fallback.
type assetHandler struct {
	files  http.Handler
	fsys   fs.FS
	static string
}

// newAssetHandler checks the root actually holds a built SPA, so a bad mount
// or a missing build stage fails at startup with a clear message rather than
// serving 404s to a browser that then shows a blank page.
func newAssetHandler(root string) (*assetHandler, error) {
	if root == "" {
		return nil, fmt.Errorf("webui: asset root is required")
	}
	fsys := os.DirFS(root)
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return nil, fmt.Errorf("webui: no index.html under %q: %w", root, err)
	}
	return &assetHandler{
		files:  http.FileServer(http.Dir(root)),
		fsys:   fsys,
		static: root,
	}, nil
}

// ServeHTTP serves a real file when one exists and index.html otherwise.
//
// The fallback is what makes a deep link like /mcps/postgres-ro work: the
// frontend has no server-side router (see the Admin UI design doc), so every
// route it owns has to arrive as index.html and be resolved in the browser.
// Only GET and HEAD fall back -- a POST to a path that does not exist is a
// mistake, and answering it with an HTML page hides that.
func (h *assetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.files.ServeHTTP(w, r)
		return
	}
	if h.exists(r.URL.Path) {
		h.files.ServeHTTP(w, r)
		return
	}
	// A missing *asset* is a 404, not the app shell. Returning index.html for
	// /assets/index-abc123.js would hand the browser HTML with a JavaScript
	// content type and produce a syntax error instead of a clear miss -- the
	// classic symptom of a stale or half-copied build.
	if path.Ext(r.URL.Path) != "" {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

// exists reports whether the request path names a real file under the root.
// fs.ValidPath rejects anything that escapes it, so a traversal attempt falls
// through to the SPA fallback rather than reaching the filesystem.
func (h *assetHandler) exists(urlPath string) bool {
	name := strings.TrimPrefix(path.Clean(urlPath), "/")
	if name == "" || name == "." {
		return false
	}
	if !fs.ValidPath(name) {
		return false
	}
	info, err := fs.Stat(h.fsys, name)
	return err == nil && !info.IsDir()
}

func (h *assetHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(h.fsys, "index.html")
	if err != nil {
		http.Error(w, "index.html is unreadable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The shell must not be cached: its whole job is to reference the current
	// hashed asset filenames, and a cached one points at a previous build's.
	// The hashed assets under /assets are immutable and cache normally.
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
