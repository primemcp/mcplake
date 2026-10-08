package router_test

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/primemcp/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyStore_DelegatesToWrappedEngine(t *testing.T) {
	accessPolicies := []router.AccessPolicy{{
		Name:    "db-reader",
		Enabled: true,
		Match:   router.ClaimMatcher{Rules: []router.ClaimRule{mustRule(t, "$.role", "^db-reader$")}},
		Grants:  []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
	}}
	filterPolicies := []router.FilterPolicy{{
		Name:       "hide-pii",
		Enabled:    true,
		Match:      router.ClaimMatcher{Rules: []router.ClaimRule{mustRule(t, "$.role", "^db-reader$")}},
		MCP:        "postgres-ro",
		Tool:       "get_user",
		DropFields: []string{"$.email"},
	}}
	engine := router.NewEngine(accessPolicies, filterPolicies)
	store := router.NewPolicyStore(engine)

	claims := json.RawMessage(`{"role":"db-reader"}`)

	authorized, err := store.Authorize(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.True(t, authorized)

	fields, err := store.FieldsToRemove(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.Equal(t, []string{"$.email"}, fields)
}

func TestPolicyStore_ReloadReplacesNotAccumulates(t *testing.T) {
	initial := router.NewEngine(
		[]router.AccessPolicy{{
			Name:    "db-reader",
			Enabled: true,
			Match:   router.ClaimMatcher{Rules: []router.ClaimRule{mustRule(t, "$.role", "^db-reader$")}},
			Grants:  []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
		}},
		nil,
	)
	store := router.NewPolicyStore(initial)
	claims := json.RawMessage(`{"role":"db-reader"}`)

	authorized, err := store.Authorize(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	require.True(t, authorized, "sanity check before reload")

	store.Reload(
		[]router.AccessPolicy{{
			Name:    "db-writer",
			Enabled: true,
			Match:   router.ClaimMatcher{Rules: []router.ClaimRule{mustRule(t, "$.role", "^db-writer$")}},
			Grants:  []router.Grant{{MCP: "postgres-rw", Tools: []string{"*"}}},
		}},
		nil,
	)

	stillOld, err := store.Authorize(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.False(t, stillOld, "old policy must no longer match after Reload")

	newClaims := json.RawMessage(`{"role":"db-writer"}`)
	nowNew, err := store.Authorize(newClaims, "postgres-rw", "get_user")
	require.NoError(t, err)
	assert.True(t, nowNew)
}

func TestPolicyStore_ConcurrentAuthorizeDuringReload(t *testing.T) {
	store := router.NewPolicyStore(router.NewEngine(nil, nil))
	claims := json.RawMessage(`{"role":"db-reader"}`)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var reads atomic.Int64

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = store.Authorize(claims, "postgres-ro", "get_user")
					_, _ = store.FieldsToRemove(claims, "postgres-ro", "get_user")
					reads.Add(1)
				}
			}
		}()
	}

	for i := 0; i < 200; i++ {
		store.Reload([]router.AccessPolicy{{
			Name:    "db-reader",
			Enabled: true,
			Match:   router.ClaimMatcher{Rules: []router.ClaimRule{mustRule(t, "$.role", "^db-reader$")}},
			Grants:  []router.Grant{{MCP: "postgres-ro", Tools: []string{"*"}}},
		}}, nil)
	}

	// Wait for the readers to actually get scheduled before stopping them.
	// Without this the 200 reloads above can finish and close(stop) before
	// any reader goroutine runs, leaving reads at 0 and failing the
	// assertion for a scheduling reason that has nothing to do with the
	// concurrent-reload behaviour under test.
	require.Eventually(t, func() bool { return reads.Load() > 0 }, 5*time.Second, time.Millisecond)

	close(stop)
	wg.Wait()
	assert.Positive(t, reads.Load())
}
