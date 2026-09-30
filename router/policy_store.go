package router

import (
	"encoding/json"
	"sync/atomic"
)

// PolicyStore wraps an *Engine behind an atomic pointer so the control-plane
// admin API (Epic #4) can swap in a freshly-loaded policy set after every
// write, without the data plane ever querying the database or observing a
// partially-updated policy set. See
// docs/architecture/components.rst#control-plane-api-gin.
//
// PolicyStore satisfies the same (Authorize, FieldsToRemove) shape as
// *Engine, so it is a drop-in replacement wherever *Engine is used today
// (e.g. gateway.Config.Policy).
type PolicyStore struct {
	engine atomic.Pointer[Engine]
}

// NewPolicyStore returns a PolicyStore initialized with engine.
func NewPolicyStore(engine *Engine) *PolicyStore {
	s := &PolicyStore{}
	s.engine.Store(engine)
	return s
}

// Authorize delegates to the currently active Engine. See Engine.Authorize.
func (s *PolicyStore) Authorize(claims json.RawMessage, mcp, tool string) (bool, error) {
	return s.engine.Load().Authorize(claims, mcp, tool)
}

// FieldsToRemove delegates to the currently active Engine. See
// Engine.FieldsToRemove.
func (s *PolicyStore) FieldsToRemove(claims json.RawMessage, mcp, tool string) ([]string, error) {
	return s.engine.Load().FieldsToRemove(claims, mcp, tool)
}

// Reload builds a new Engine from accessPolicies/filterPolicies and
// atomically swaps it in, fully replacing (not merging with) the previous
// policy set. A concurrent Authorize/FieldsToRemove call in flight completes
// against whichever Engine — old or new — it already loaded; it never
// observes a partially-updated policy set, since each Engine is immutable
// once constructed and the swap itself is a single atomic pointer store.
func (s *PolicyStore) Reload(accessPolicies []AccessPolicy, filterPolicies []FilterPolicy) {
	s.engine.Store(NewEngine(accessPolicies, filterPolicies))
}
