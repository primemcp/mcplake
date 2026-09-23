package app_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/atsokha/mcplake/config"
	"github.com/atsokha/mcplake/internal"
	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// e2eBearerTransport attaches a token to every request an MCP client
// transport makes, which is how a real client authenticates to the gateway.
type e2eBearerTransport struct {
	token string
}

func (b *e2eBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(clone)
}

// connectGatewayAsMCPClient dials the running gateway the way an editor or
// an agent would: as an MCP server, over one of its two HTTP transports.
func connectGatewayAsMCPClient(t *testing.T, ctx context.Context, transport sdk.Transport) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-e2e-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// The acceptance criterion for ADR-0020, end to end through the real
// binary's wiring: an off-the-shelf MCP client points at the gateway, over
// either transport, and gets back a tool catalogue filtered to what its
// token authorizes — then calls one of those tools and receives a result the
// filter policy has already redacted.
//
// Everything here is the production path: config seeding, the real
// cache.Registry, a real JWT validated against a real JWKS endpoint, the
// real router.Engine, and a downstream MCP the gateway does not run.
func TestApp_MCPEndpoint_EndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	cfg := testConfig(t, jwks.URL)
	cfg.MCPs = []config.MCPConfig{{
		Name: "remote-directory",
		Type: "http",
		URL:  startRemoteMCP(t),
	}}
	cfg.AccessPolicies = []config.AccessPolicyConfig{{
		Name:   "db-reader",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
		Grants: []config.GrantConfig{{MCP: "remote-directory", Tools: []string{"*"}}},
	}}
	cfg.FilterPolicies = []config.FilterPolicyConfig{{
		Name:       "hide-salary",
		Match:      []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
		MCP:        "remote-directory",
		Tool:       "get_employee",
		DropFields: []string{"$.salary_usd"},
	}}

	dataAddr, _, cleanup := startTestApp(t, cfg)
	defer cleanup()

	now := time.Now()
	token := signToken(t, key, jwt.MapClaims{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "alice",
		"role": "db-reader",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})
	httpClient := &http.Client{Transport: &e2eBearerTransport{token: token}}
	base := "http://" + dataAddr

	transports := map[string]sdk.Transport{
		"streamable": &sdk.StreamableClientTransport{
			Endpoint:   base + internal.MCPStreamablePath,
			HTTPClient: httpClient,
		},
		"sse": &sdk.SSEClientTransport{
			Endpoint:   base + internal.MCPSSEPath,
			HTTPClient: httpClient,
		},
	}

	for name, transport := range transports {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			session := connectGatewayAsMCPClient(t, ctx, transport)

			listed, err := session.ListTools(ctx, nil)
			require.NoError(t, err)
			require.Len(t, listed.Tools, 1)
			assert.Equal(t, "remote-directory__get_employee", listed.Tools[0].Name,
				"tools are namespaced by the MCP they belong to")
			assert.Equal(t, "returns one employee record", listed.Tools[0].Description,
				"the downstream's description has to survive discovery for a client to pick the tool")

			result, err := session.CallTool(ctx, &sdk.CallToolParams{
				Name: "remote-directory__get_employee",
			})
			require.NoError(t, err)
			require.False(t, result.IsError)

			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), "Marcus Webb",
				"the remote MCP's response should come through")
			assert.NotContains(t, string(encoded), "salary_usd",
				"drop_fields must apply on the MCP surface exactly as on POST /v1/call")
			assert.NotContains(t, string(encoded), "198000")
		})
	}
}

// A caller whose claims grant nothing sees an empty catalogue rather than a
// full one it would be refused on: discovery and enforcement are the same
// access decision. This is the property POST /v1/call could not express at
// all, since it has nothing to discover.
func TestApp_MCPEndpoint_CatalogueIsFilteredByTheCallersGrants(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	cfg := testConfig(t, jwks.URL)
	cfg.MCPs = []config.MCPConfig{{
		Name: "remote-directory",
		Type: "http",
		URL:  startRemoteMCP(t),
	}}
	cfg.AccessPolicies = []config.AccessPolicyConfig{{
		Name:   "db-reader",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
		Grants: []config.GrantConfig{{MCP: "remote-directory", Tools: []string{"*"}}},
	}}

	dataAddr, _, cleanup := startTestApp(t, cfg)
	defer cleanup()

	now := time.Now()
	guestToken := signToken(t, key, jwt.MapClaims{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "bob",
		"role": "guest",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	session := connectGatewayAsMCPClient(t, ctx, &sdk.StreamableClientTransport{
		Endpoint:   "http://" + dataAddr + internal.MCPStreamablePath,
		HTTPClient: &http.Client{Transport: &e2eBearerTransport{token: guestToken}},
	})

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, listed.Tools, "a caller with no grant must see no tools")

	// And the same caller is still refused if it calls the tool anyway,
	// having learned the name some other way.
	_, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "remote-directory__get_employee"})
	require.Error(t, err, "an empty catalogue must not be the only thing standing between a caller and a tool")
}
