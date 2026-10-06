package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// healthCheckInterval is short enough to keep these tests quick and long
// enough that a tick is not racing the assertions around it. The production
// default is cache.DefaultHealthCheckInterval; the loop's scheduling and
// backoff are covered in cache, so what these tests are for is the wiring
// and the real transport.
const healthCheckInterval = 100 * time.Millisecond

// restartableMCP is a Streamable HTTP MCP server on a fixed loopback
// address that a test can stop and start again, standing in for the thing
// ADR-0019 is about: a downstream the gateway neither owns nor is told
// about when it goes away and comes back. The address is held across the
// restart so the registration's URL stays valid, exactly as a container
// that is restarted in place keeps its endpoint.
type restartableMCP struct {
	addr string
	srv  *http.Server
}

func newRestartableMCP(t *testing.T) *restartableMCP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	m := &restartableMCP{addr: ln.Addr().String()}
	m.serve(t, ln)
	t.Cleanup(m.stop)
	return m
}

// newStoppedMCP reserves a loopback address without serving on it, for the
// "registered before its server exists" case.
func newStoppedMCP(t *testing.T) *restartableMCP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	m := &restartableMCP{addr: addr}
	t.Cleanup(m.stop)
	return m
}

func (m *restartableMCP) serve(t *testing.T, ln net.Listener) {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-e2e-remote", Version: "0.1.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_employee",
		Description: "returns one employee record",
	}, func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, employeeRecord, error) {
		return nil, employeeRecord{Name: "Marcus Webb", SalaryUSD: 198000}, nil
	})

	m.srv = &http.Server{
		Handler: sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil),
	}
	go func() { _ = m.srv.Serve(ln) }()
}

// stop closes the listener and every live connection, so the gateway's
// session dies the way it would if the process behind it exited.
func (m *restartableMCP) stop() {
	if m.srv != nil {
		_ = m.srv.Close()
		m.srv = nil
	}
}

// start serves again on the same address, with a fresh MCP server that has
// never heard of the gateway's previous session.
func (m *restartableMCP) start(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", m.addr)
	require.NoError(t, err)
	m.serve(t, ln)
}

func (m *restartableMCP) url() string { return "http://" + m.addr }

// remoteMCPConfig is a gateway config with one http MCP at url, callable by
// the admin role, and the health loop running fast.
func remoteMCPConfig(t *testing.T, jwksURL, url string) *config.Config {
	t.Helper()
	cfg := testConfig(t, jwksURL)
	cfg.MCP.HealthCheckInterval = &config.Duration{Duration: healthCheckInterval}
	cfg.MCPs = []config.MCPConfig{{
		Name: "remote-directory",
		Type: "http",
		URL:  url,
	}}
	cfg.AccessPolicies = []config.AccessPolicyConfig{{
		Name:   "db-reader",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
		Grants: []config.GrantConfig{{MCP: "remote-directory", Tools: []string{"*"}}},
	}}
	return cfg
}

func dbReaderToken(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	now := time.Now()
	return signToken(t, key, jwt.MapClaims{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "alice",
		"role": "db-reader",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})
}

// callRemote posts one tool call and returns the status and body.
func callRemote(t *testing.T, dataAddr, token string) (int, string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"mcp": "remote-directory", "tool": "get_employee"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", dataAddr), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(out)
}

// awaitCallable polls until a tool call succeeds, returning how long it
// took. It fails the test rather than returning an error, since every
// caller's next step depends on it.
func awaitCallable(t *testing.T, dataAddr, token string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastStatus int
	var lastBody string
	for time.Now().Before(deadline) {
		lastStatus, lastBody = callRemote(t, dataAddr, token)
		if lastStatus == http.StatusOK {
			assert.Contains(t, lastBody, "Marcus Webb")
			return
		}
		time.Sleep(healthCheckInterval / 2)
	}
	t.Fatalf("mcp never became callable; last response %d: %s", lastStatus, lastBody)
}

// The acceptance criterion for #191: an http MCP whose server restarts is
// serving calls again without anybody restarting the gateway.
//
// Before ADR-0019 this test's final call returned 502 forever — the
// registration kept a session pointing at a process that no longer existed,
// and nothing in the gateway was capable of noticing or rebuilding it.
func TestApp_RemoteMCPRecoversFromADownstreamRestart(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	remote := newRestartableMCP(t)
	dataAddr, _, cleanup := startTestApp(t, remoteMCPConfig(t, jwks.URL, remote.url()))
	defer cleanup()

	token := dbReaderToken(t, key)

	status, body := callRemote(t, dataAddr, token)
	require.Equal(t, http.StatusOK, status, "body: %s", body)

	remote.stop()

	// While it is down the call must fail — and must say the MCP is
	// unavailable, not that it does not exist.
	status, body = callRemote(t, dataAddr, token)
	assert.NotEqual(t, http.StatusOK, status, "a call to a stopped downstream must not succeed")
	assert.NotEqual(t, http.StatusNotFound, status,
		"a registered MCP that is merely down must not read as not found; body: %s", body)

	remote.start(t)

	awaitCallable(t, dataAddr, token)
}

// The other half: an MCP that was not listening when the gateway started up
// becomes usable on its own once its server arrives. Before ADR-0019 the
// registration stayed unreachable with zero tools until an operator
// re-POSTed it.
func TestApp_RemoteMCPRegisteredBeforeItsServerExistsRecovers(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	remote := newStoppedMCP(t)
	dataAddr, _, cleanup := startTestApp(t, remoteMCPConfig(t, jwks.URL, remote.url()))
	defer cleanup()

	token := dbReaderToken(t, key)

	status, body := callRemote(t, dataAddr, token)
	require.Equal(t, http.StatusServiceUnavailable, status,
		"a registration whose downstream never answered is unavailable, not missing; body: %s", body)

	remote.start(t)

	awaitCallable(t, dataAddr, token)
}

// A downstream that stays down must not be reported as healthy, and the
// registration must not keep a client that cannot carry a call.
func TestApp_RemoteMCPThatStaysDownIsReportedUnreachable(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	remote := newRestartableMCP(t)
	dataAddr, controlAddr, cleanup := startTestApp(t, remoteMCPConfig(t, jwks.URL, remote.url()))
	defer cleanup()

	token := dbReaderToken(t, key)
	status, _ := callRemote(t, dataAddr, token)
	require.Equal(t, http.StatusOK, status)

	remote.stop()

	deadline := time.Now().Add(10 * time.Second)
	var reg map[string]any
	for time.Now().Before(deadline) {
		reg = fetchRemoteRegistration(t, controlAddr)
		if reg["status"] == "unreachable" {
			break
		}
		time.Sleep(healthCheckInterval / 2)
	}
	require.Equal(t, "unreachable", reg["status"],
		"an MCP whose session is dead must not keep reporting itself active")

	// The cached schemas survive the outage: Status is what makes the MCP
	// uncallable, and the admin UI still needs the tool list to show and
	// edit filters for an endpoint that is merely down.
	tools, _ := reg["tools"].(map[string]any)
	assert.Contains(t, tools, "get_employee")

	status, body := callRemote(t, dataAddr, token)
	assert.Equal(t, http.StatusServiceUnavailable, status, "body: %s", body)
}

func fetchRemoteRegistration(t *testing.T, controlAddr string) map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/admin/mcps", controlAddr))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var regs []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&regs))
	for _, reg := range regs {
		if reg["name"] == "remote-directory" {
			return reg
		}
	}
	t.Fatal("remote-directory is not registered")
	return nil
}
