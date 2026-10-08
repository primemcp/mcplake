package adminservice

import (
	"context"
	"fmt"

	"github.com/primemcp/mcplake/router"
)

// AccessPolicyService performs the access-policy CRUD behind
// /admin/access-policies. Every write persists via the repository and then
// refreshes the shared live policy engine, so the change is immediately
// visible to the data plane's authorization checks.
type AccessPolicyService struct {
	repo     AccessPolicyRepository
	reloader Reloader
}

// NewAccessPolicyService returns a service backed by repo (durable) and
// reloader (live engine refresh).
func NewAccessPolicyService(repo AccessPolicyRepository, reloader Reloader) *AccessPolicyService {
	return &AccessPolicyService{repo: repo, reloader: reloader}
}

// List returns every stored access policy.
func (s *AccessPolicyService) List(ctx context.Context) ([]router.AccessPolicy, error) {
	return s.repo.List(ctx)
}

// Get returns one stored access policy, or ErrNotFound.
func (s *AccessPolicyService) Get(ctx context.Context, name string) (router.AccessPolicy, error) {
	policy, ok, err := s.repo.Get(ctx, name)
	if err != nil {
		return router.AccessPolicy{}, err
	}
	if !ok {
		return router.AccessPolicy{}, fmt.Errorf("%w: access policy %q", ErrNotFound, name)
	}
	return policy, nil
}

// Upsert stores policy (creating or replacing it) and refreshes the live
// engine. A store rejection (e.g. an uncompilable claim rule) is
// ErrInvalidPolicy and nothing is persisted.
func (s *AccessPolicyService) Upsert(ctx context.Context, policy router.AccessPolicy) error {
	if err := s.repo.Upsert(ctx, policy); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPolicy, err)
	}
	return s.reloader.Refresh(ctx)
}

// Delete removes the named access policy and refreshes the live engine.
// Unknown name is ErrNotFound.
func (s *AccessPolicyService) Delete(ctx context.Context, name string) error {
	_, ok, err := s.repo.Get(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: access policy %q", ErrNotFound, name)
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return fmt.Errorf("delete access policy %q: %w", name, err)
	}
	return s.reloader.Refresh(ctx)
}

// FilterPolicyService performs the filter-policy CRUD behind
// /admin/filter-policies, with the same persist-then-refresh behavior as
// AccessPolicyService.
type FilterPolicyService struct {
	repo     FilterPolicyRepository
	reloader Reloader
}

// NewFilterPolicyService returns a service backed by repo (durable) and
// reloader (live engine refresh).
func NewFilterPolicyService(repo FilterPolicyRepository, reloader Reloader) *FilterPolicyService {
	return &FilterPolicyService{repo: repo, reloader: reloader}
}

// List returns every stored filter policy.
func (s *FilterPolicyService) List(ctx context.Context) ([]router.FilterPolicy, error) {
	return s.repo.List(ctx)
}

// Get returns one stored filter policy, or ErrNotFound.
func (s *FilterPolicyService) Get(ctx context.Context, name string) (router.FilterPolicy, error) {
	policy, ok, err := s.repo.Get(ctx, name)
	if err != nil {
		return router.FilterPolicy{}, err
	}
	if !ok {
		return router.FilterPolicy{}, fmt.Errorf("%w: filter policy %q", ErrNotFound, name)
	}
	return policy, nil
}

// Upsert stores policy (creating or replacing it) and refreshes the live
// engine. A store rejection is ErrInvalidPolicy and nothing is persisted.
func (s *FilterPolicyService) Upsert(ctx context.Context, policy router.FilterPolicy) error {
	if err := s.repo.Upsert(ctx, policy); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPolicy, err)
	}
	return s.reloader.Refresh(ctx)
}

// Delete removes the named filter policy and refreshes the live engine.
// Unknown name is ErrNotFound.
func (s *FilterPolicyService) Delete(ctx context.Context, name string) error {
	_, ok, err := s.repo.Get(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: filter policy %q", ErrNotFound, name)
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return fmt.Errorf("delete filter policy %q: %w", name, err)
	}
	return s.reloader.Refresh(ctx)
}
