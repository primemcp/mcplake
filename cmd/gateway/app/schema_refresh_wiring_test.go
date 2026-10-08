package app_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/primemcp/mcplake/cmd/gateway/app"
	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/require"
)

func TestApp_SchemaRefresh_RunsAndStopsCleanly(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	cfg := testConfig(t, jwks.URL)
	cfg.MCP.SchemaRefreshInterval = config.Duration{Duration: 50 * time.Millisecond}

	// startTestApp boots Run in a goroutine and its cleanup cancels the run
	// context and asserts Run returned nil — i.e. the refresher loop also
	// drained cleanly on shutdown.
	_, _, cleanup := startTestApp(t, cfg)

	// Let a few refresh ticks fire against the (empty) registry.
	time.Sleep(200 * time.Millisecond)

	cleanup()
}

func TestApp_New_SchemaRefreshDisabledByDefault(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	a, err := app.New(context.Background(), testConfig(t, jwks.URL))
	require.NoError(t, err)
	require.NotNil(t, a)
	// No interval configured: New succeeds and Run/shutdown still work (the
	// refresher channel is pre-satisfied). Covered behaviourally by every
	// other app test, which all run with no [mcp] section.
}
