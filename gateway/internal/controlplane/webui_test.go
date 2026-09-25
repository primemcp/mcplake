package controlplane_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startUITestServer starts a Server with RegisterUIRoutes wired against a
// fixture filesystem (not the real embedded build), so these tests exercise
// routing behavior independent of whatever webui/dist currently contains.
func startUITestServer(t testing.TB, assets fstest.MapFS) (addr string, cleanup func()) {
	t.Helper()

	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	controlplane.RegisterUIRoutes(s.Engine(), assets)

	startErr := make(chan error, 1)
	go func() { startErr <- s.Start(context.Background()) }()

	select {
	case <-s.Ready():
	case err := <-startErr:
		t.Fatalf("server failed to start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server readiness")
	}

	cleanup = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))

		select {
		case err := <-startErr:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not return after Stop")
		}
	}
	return s.Addr(), cleanup
}

func fixtureAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>root</html>")},
		"assets/app.js": {Data: []byte("console.log('hi')")},
	}
}

func TestRegisterUIRoutes_RootServesIndexHTML(t *testing.T) {
	addr, cleanup := startUITestServer(t, fixtureAssets())
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "root")
}

func TestRegisterUIRoutes_IndexHTMLIsNotCached(t *testing.T) {
	addr, cleanup := startUITestServer(t, fixtureAssets())
	defer cleanup()

	for _, p := range []string{"/", "/index.html", "/mcps/some-mcp"} {
		resp, err := http.Get(fmt.Sprintf("http://%s%s", addr, p))
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"), p)
	}
}

func TestRegisterUIRoutes_KnownAssetServedDirectly(t *testing.T) {
	addr, cleanup := startUITestServer(t, fixtureAssets())
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/assets/app.js", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "console.log")
}

func TestRegisterUIRoutes_UnmatchedNonAdminPathFallsBackToIndexHTML(t *testing.T) {
	addr, cleanup := startUITestServer(t, fixtureAssets())
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/some/deep/link", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "root")
}

func TestRegisterUIRoutes_UnmatchedAdminPathStill404s(t *testing.T) {
	addr, cleanup := startUITestServer(t, fixtureAssets())
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/admin/nope", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestWebUIAssets_ContainsIndexHTML(t *testing.T) {
	f, err := controlplane.WebUIAssets.Open("index.html")
	require.NoError(t, err)
	defer f.Close()
}
