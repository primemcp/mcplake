package adminservice_test

import (
	"context"
	"errors"
	"testing"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/atsokha/mcplake/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes -----------------------------------------------------------------

type fakeRegistry struct {
	regs        map[string]cache.MCPRegistration
	registerErr error
	unregErr    error
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{regs: map[string]cache.MCPRegistration{}}
}

func (f *fakeRegistry) Register(_ context.Context, reg cache.MCPRegistration) error {
	if f.registerErr != nil {
		return f.registerErr
	}
	reg.Status = cache.StatusActive
	reg.Tools = map[string]cache.ToolSchema{"t": {Name: "t"}}
	f.regs[reg.Name] = reg
	return nil
}

func (f *fakeRegistry) Unregister(name string) error {
	if f.unregErr != nil {
		return f.unregErr
	}
	if _, ok := f.regs[name]; !ok {
		return errors.New("cache: not registered")
	}
	delete(f.regs, name)
	return nil
}

func (f *fakeRegistry) Get(name string) (cache.MCPRegistration, bool) {
	reg, ok := f.regs[name]
	return reg, ok
}

func (f *fakeRegistry) List() []cache.MCPRegistration {
	out := make([]cache.MCPRegistration, 0, len(f.regs))
	for _, r := range f.regs {
		out = append(out, r)
	}
	return out
}

func (f *fakeRegistry) SetEnabled(name string, enabled bool) (cache.MCPRegistration, bool) {
	reg, ok := f.regs[name]
	if !ok {
		return cache.MCPRegistration{}, false
	}
	reg.Enabled = enabled
	f.regs[name] = reg
	return reg, true
}

type fakeMCPRepo struct {
	rows      map[string]cache.MCPRegistration
	upsertErr error
	deleteErr error
}

func newFakeMCPRepo() *fakeMCPRepo { return &fakeMCPRepo{rows: map[string]cache.MCPRegistration{}} }

func (f *fakeMCPRepo) Upsert(_ context.Context, reg cache.MCPRegistration) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[reg.Name] = reg
	return nil
}

func (f *fakeMCPRepo) Delete(_ context.Context, name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.rows, name)
	return nil
}

type fakeAccessRepo struct {
	policies  map[string]router.AccessPolicy
	upsertErr error
	getErr    error
}

func newFakeAccessRepo() *fakeAccessRepo {
	return &fakeAccessRepo{policies: map[string]router.AccessPolicy{}}
}

func (f *fakeAccessRepo) Upsert(_ context.Context, p router.AccessPolicy) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.policies[p.Name] = p
	return nil
}

func (f *fakeAccessRepo) Get(_ context.Context, name string) (router.AccessPolicy, bool, error) {
	if f.getErr != nil {
		return router.AccessPolicy{}, false, f.getErr
	}
	p, ok := f.policies[name]
	return p, ok, nil
}

func (f *fakeAccessRepo) List(_ context.Context) ([]router.AccessPolicy, error) {
	out := make([]router.AccessPolicy, 0, len(f.policies))
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeAccessRepo) Delete(_ context.Context, name string) error {
	delete(f.policies, name)
	return nil
}

type fakeReloader struct {
	calls int
	err   error
}

func (f *fakeReloader) Refresh(context.Context) error {
	f.calls++
	return f.err
}

// --- MCPService ----------------------------------------------------------

func TestMCPService_Register_PersistsRegistryResultAndDefaultsTransport(t *testing.T) {
	reg := newFakeRegistry()
	repo := newFakeMCPRepo()
	svc := adminservice.NewMCPService(reg, repo)

	got, err := svc.Register(context.Background(), cache.MCPRegistration{Name: "pg"})
	require.NoError(t, err)

	assert.Equal(t, adminservice.DefaultMCPTransport, got.Transport)
	assert.Equal(t, cache.StatusActive, got.Status)
	assert.NotEmpty(t, got.Tools, "the persisted row is the post-discovery registry state")
	assert.Equal(t, got, repo.rows["pg"])
}

func TestMCPService_Register_EmptyNameIsInvalidRequest(t *testing.T) {
	svc := adminservice.NewMCPService(newFakeRegistry(), newFakeMCPRepo())

	_, err := svc.Register(context.Background(), cache.MCPRegistration{})

	assert.ErrorIs(t, err, adminservice.ErrInvalidRequest)
}

func TestMCPService_Register_RegistryFailureIsRegistrationFailedAndPersistsNothing(t *testing.T) {
	reg := newFakeRegistry()
	reg.registerErr = errors.New("connection refused")
	repo := newFakeMCPRepo()
	svc := adminservice.NewMCPService(reg, repo)

	_, err := svc.Register(context.Background(), cache.MCPRegistration{Name: "pg"})

	assert.ErrorIs(t, err, adminservice.ErrRegistrationFailed)
	assert.Empty(t, repo.rows)
}

func TestMCPService_Register_RepoFailureIsInternal(t *testing.T) {
	repo := newFakeMCPRepo()
	repo.upsertErr = errors.New("disk full")
	svc := adminservice.NewMCPService(newFakeRegistry(), repo)

	_, err := svc.Register(context.Background(), cache.MCPRegistration{Name: "pg"})

	require.Error(t, err)
	assert.NotErrorIs(t, err, adminservice.ErrRegistrationFailed)
	assert.NotErrorIs(t, err, adminservice.ErrInvalidRequest)
	assert.NotErrorIs(t, err, adminservice.ErrNotFound)
}

func TestMCPService_SetEnabled_UnknownNameIsNotFound(t *testing.T) {
	svc := adminservice.NewMCPService(newFakeRegistry(), newFakeMCPRepo())

	_, err := svc.SetEnabled(context.Background(), "nope", false)

	assert.ErrorIs(t, err, adminservice.ErrNotFound)
}

func TestMCPService_SetEnabled_TogglesAndPersists(t *testing.T) {
	reg := newFakeRegistry()
	repo := newFakeMCPRepo()
	svc := adminservice.NewMCPService(reg, repo)
	_, err := svc.Register(context.Background(), cache.MCPRegistration{Name: "pg", Enabled: true})
	require.NoError(t, err)

	got, err := svc.SetEnabled(context.Background(), "pg", false)
	require.NoError(t, err)

	assert.False(t, got.Enabled)
	assert.False(t, repo.rows["pg"].Enabled)
}

func TestMCPService_Unregister_UnknownNameIsNotFound(t *testing.T) {
	svc := adminservice.NewMCPService(newFakeRegistry(), newFakeMCPRepo())

	err := svc.Unregister(context.Background(), "nope")

	assert.ErrorIs(t, err, adminservice.ErrNotFound)
}

func TestMCPService_Unregister_RemovesFromRegistryAndRepo(t *testing.T) {
	reg := newFakeRegistry()
	repo := newFakeMCPRepo()
	svc := adminservice.NewMCPService(reg, repo)
	_, err := svc.Register(context.Background(), cache.MCPRegistration{Name: "pg"})
	require.NoError(t, err)

	require.NoError(t, svc.Unregister(context.Background(), "pg"))

	_, ok := reg.Get("pg")
	assert.False(t, ok)
	_, ok = repo.rows["pg"]
	assert.False(t, ok)
}

// --- AccessPolicyService ----------------------------------------------------

func TestAccessPolicyService_Upsert_PersistsAndRefreshes(t *testing.T) {
	repo := newFakeAccessRepo()
	rl := &fakeReloader{}
	svc := adminservice.NewAccessPolicyService(repo, rl)

	err := svc.Upsert(context.Background(), router.AccessPolicy{Name: "p"})
	require.NoError(t, err)

	assert.Contains(t, repo.policies, "p")
	assert.Equal(t, 1, rl.calls)
}

func TestAccessPolicyService_Upsert_RepoRejectionIsInvalidPolicyAndSkipsRefresh(t *testing.T) {
	repo := newFakeAccessRepo()
	repo.upsertErr = errors.New("bad rule")
	rl := &fakeReloader{}
	svc := adminservice.NewAccessPolicyService(repo, rl)

	err := svc.Upsert(context.Background(), router.AccessPolicy{Name: "p"})

	assert.ErrorIs(t, err, adminservice.ErrInvalidPolicy)
	assert.Zero(t, rl.calls)
}

func TestAccessPolicyService_Upsert_RefreshErrorPropagates(t *testing.T) {
	rl := &fakeReloader{err: errors.New("reload failed")}
	svc := adminservice.NewAccessPolicyService(newFakeAccessRepo(), rl)

	err := svc.Upsert(context.Background(), router.AccessPolicy{Name: "p"})

	require.Error(t, err)
	assert.NotErrorIs(t, err, adminservice.ErrInvalidPolicy)
}

func TestAccessPolicyService_Get_UnknownIsNotFound(t *testing.T) {
	svc := adminservice.NewAccessPolicyService(newFakeAccessRepo(), &fakeReloader{})

	_, err := svc.Get(context.Background(), "nope")

	assert.ErrorIs(t, err, adminservice.ErrNotFound)
}

func TestAccessPolicyService_Get_RepoErrorPropagates(t *testing.T) {
	repo := newFakeAccessRepo()
	repo.getErr = errors.New("db down")
	svc := adminservice.NewAccessPolicyService(repo, &fakeReloader{})

	_, err := svc.Get(context.Background(), "p")

	require.Error(t, err)
	assert.NotErrorIs(t, err, adminservice.ErrNotFound)
}

func TestAccessPolicyService_Delete_UnknownIsNotFoundAndSkipsRefresh(t *testing.T) {
	rl := &fakeReloader{}
	svc := adminservice.NewAccessPolicyService(newFakeAccessRepo(), rl)

	err := svc.Delete(context.Background(), "nope")

	assert.ErrorIs(t, err, adminservice.ErrNotFound)
	assert.Zero(t, rl.calls)
}

func TestAccessPolicyService_Delete_RemovesAndRefreshes(t *testing.T) {
	repo := newFakeAccessRepo()
	rl := &fakeReloader{}
	svc := adminservice.NewAccessPolicyService(repo, rl)
	require.NoError(t, svc.Upsert(context.Background(), router.AccessPolicy{Name: "p"}))

	require.NoError(t, svc.Delete(context.Background(), "p"))

	assert.NotContains(t, repo.policies, "p")
	assert.Equal(t, 2, rl.calls, "one refresh for the upsert, one for the delete")
}
