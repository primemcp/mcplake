package mcp

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The transports a downstream MCP can be reached over. These are the
// canonical spellings: they are persisted verbatim in the MCPRegistration
// store, accepted verbatim by the admin API, and written verbatim in
// `[[mcps]]`'s `type` key, so they are a wire format -- renaming one
// orphans existing rows.
const (
	// TransportStdio spawns the server as a subprocess and speaks over its
	// stdin/stdout. The server's lifetime is the gateway's.
	TransportStdio = "stdio"
	// TransportHTTP is the MCP Streamable HTTP transport: the current
	// standard for a server the gateway does not itself run.
	TransportHTTP = "http"
	// TransportSSE is the older HTTP+SSE transport, kept because plenty of
	// deployed servers still only speak it.
	TransportSSE = "sse"
)

// SupportedTransports lists every transport NewClient can build, in the
// order a UI should offer them.
func SupportedTransports() []string {
	return []string{TransportStdio, TransportHTTP, TransportSSE}
}

// TransportSupported reports whether transport names one NewClient can
// build. The empty string counts: it means stdio, for callers written
// before there was anything else.
func TransportSupported(transport string) bool {
	switch transport {
	case "", TransportStdio, TransportHTTP, TransportSSE:
		return true
	}
	return false
}

// httpTransportClient is the HTTP client the http/sse transports dial with.
//
// It deliberately sets no overall http.Client.Timeout. The streamable
// transport holds a long-lived GET open to receive server-initiated
// messages, and a session outlives any single request, so a whole-request
// deadline would sever a healthy connection on a timer. The bounds that
// matter for a stuck *peer* are set per phase instead: a connect, a TLS
// handshake and a response-header wait all fail fast, while a stream that
// has begun is allowed to stay open.
var httpTransportClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	},
}

// ValidateEndpointURL accepts an absolute https URL, or an absolute http
// URL whose host is loopback, and rejects everything else.
//
// This is the same rule config applies to `oidc.jwks_url` and the
// `[admin_auth.login]` endpoints (config.validateSecureHTTPURL), for a
// related reason: a tool call's arguments and its response are precisely
// the payloads this gateway exists to control access to, and shipping them
// in plaintext to a host that isn't the local machine would undercut the
// filter policies applied to them one layer up. The two implementations are
// kept in step by a test in config that runs both over the same table --
// change one and that test tells you about the other.
//
// It lives here, not in config, because config is not the only write path:
// POST /admin/mcps and the ADR-0011 MCP control server construct a
// registration directly, and all three funnel through NewClient.
//
// Loopback keeps plaintext for the same reason it does elsewhere -- local
// development, the test fixtures, and a sidecar sharing the gateway's
// network namespace all serve over an interface no attacker sits on.
func ValidateEndpointURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must be an absolute http(s) URL (got %q)", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("must include a host (got %q)", raw)
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("must use https (got %q); plaintext http is accepted only for a loopback host", raw)
	}
	return nil
}

// isLoopbackHost reports whether host names the local machine. url.URL's
// Hostname strips the port and the brackets around an IPv6 literal, so
// "[::1]:9999" arrives here as "::1". The "localhost" comparison is
// case-insensitive because hostnames are (RFC 4343) and url.URL performs no
// normalization.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
