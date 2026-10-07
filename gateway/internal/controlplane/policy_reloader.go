package controlplane

import (
	"context"
	"fmt"

	"github.com/primemcp/mcplake/router"
)

// accessPolicyRepository is the behavior the access-policy admin handlers
// need from durable storage, satisfied by *persistence.AccessPolicyRepo.
type accessPolicyRepository interface {
	Upsert(ctx context.Context, policy router.AccessPolicy) error
	Get(ctx context.Context, name string) (router.AccessPolicy, bool, error)
	List(ctx context.Context) ([]router.AccessPolicy, error)
	Delete(ctx context.Context, name string) error
}

// filterPolicyRepository is the behavior the filter-policy admin handlers
// need from durable storage, satisfied by *persistence.FilterPolicyRepo.
type filterPolicyRepository interface {
	Upsert(ctx context.Context, policy router.FilterPolicy) error
	Get(ctx context.Context, name string) (router.FilterPolicy, bool, error)
	List(ctx context.Context) ([]router.FilterPolicy, error)
	Delete(ctx context.Context, name string) error
}

// policyEngine is the behavior needed to refresh the live policy engine,
// satisfied by *router.PolicyStore.
type policyEngine interface {
	Reload(accessPolicies []router.AccessPolicy, filterPolicies []router.FilterPolicy)
}

// PolicyReloader refreshes engine from the current state of both policy
// repositories. router.PolicyStore.Reload always replaces the whole engine
// (it's built from access and filter policies together), so both the
// access-policy and filter-policy admin handlers share one reloader and
// call Refresh after every write, regardless of which policy kind changed.
type PolicyReloader struct {
	accessRepo accessPolicyRepository
	filterRepo filterPolicyRepository
	engine     policyEngine
}

// NewPolicyReloader returns a PolicyReloader that refreshes engine from
// accessRepo/filterRepo.
func NewPolicyReloader(accessRepo accessPolicyRepository, filterRepo filterPolicyRepository, engine policyEngine) *PolicyReloader {
	return &PolicyReloader{accessRepo: accessRepo, filterRepo: filterRepo, engine: engine}
}

// Refresh reads every access and filter policy currently in storage and
// atomically swaps them into the live engine.
func (r *PolicyReloader) Refresh(ctx context.Context) error {
	accessPolicies, err := r.accessRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("controlplane: list access policies: %w", err)
	}
	filterPolicies, err := r.filterRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("controlplane: list filter policies: %w", err)
	}
	r.engine.Reload(accessPolicies, filterPolicies)
	return nil
}
