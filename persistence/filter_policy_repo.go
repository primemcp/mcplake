package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/atsokha/mcplake/router"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FilterPolicyRepo persists router.FilterPolicy records via FilterPolicyRow.
type FilterPolicyRepo struct {
	db *gorm.DB
}

// NewFilterPolicyRepo returns a repository backed by db. db is expected to
// already have run AutoMigrate (see Open).
func NewFilterPolicyRepo(db *gorm.DB) *FilterPolicyRepo {
	return &FilterPolicyRepo{db: db}
}

// Upsert inserts policy, or updates the existing row with the same Name in
// place. A Match rule that fails to compile (router.NewClaimRule) is
// rejected here — the policy is not persisted.
func (r *FilterPolicyRepo) Upsert(ctx context.Context, policy router.FilterPolicy) error {
	if err := validateClaimMatcher(policy.Match); err != nil {
		return fmt.Errorf("persistence: upsert filter policy %q: %w", policy.Name, err)
	}

	row, err := filterPolicyRowFrom(policy)
	if err != nil {
		return err
	}

	err = r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"match", "mcp", "tool", "drop_fields", "updated_at"}),
		}).
		Create(&row).Error
	if err != nil {
		return fmt.Errorf("persistence: upsert filter policy %q: %w", policy.Name, err)
	}
	return nil
}

// Get returns the policy stored under name, if any, with its ClaimMatcher
// already recompiled and directly usable. found is false (with a nil error)
// when no such policy exists.
func (r *FilterPolicyRepo) Get(ctx context.Context, name string) (policy router.FilterPolicy, found bool, err error) {
	var row FilterPolicyRow
	err = r.db.WithContext(ctx).First(&row, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return router.FilterPolicy{}, false, nil
	}
	if err != nil {
		return router.FilterPolicy{}, false, fmt.Errorf("persistence: get filter policy %q: %w", name, err)
	}

	policy, err = filterPolicyFrom(row)
	if err != nil {
		return router.FilterPolicy{}, false, err
	}
	return policy, true, nil
}

// List returns every stored filter policy.
func (r *FilterPolicyRepo) List(ctx context.Context) ([]router.FilterPolicy, error) {
	var rows []FilterPolicyRow
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("persistence: list filter policies: %w", err)
	}

	policies := make([]router.FilterPolicy, 0, len(rows))
	for _, row := range rows {
		policy, err := filterPolicyFrom(row)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, nil
}

// Delete permanently removes the policy stored under name (hard delete, see
// MCPRegistrationRepo.Delete for why).
func (r *FilterPolicyRepo) Delete(ctx context.Context, name string) error {
	err := r.db.WithContext(ctx).Unscoped().Where("name = ?", name).Delete(&FilterPolicyRow{}).Error
	if err != nil {
		return fmt.Errorf("persistence: delete filter policy %q: %w", name, err)
	}
	return nil
}

func filterPolicyRowFrom(policy router.FilterPolicy) (FilterPolicyRow, error) {
	match, err := marshalClaimMatcher(policy.Match)
	if err != nil {
		return FilterPolicyRow{}, err
	}
	dropFields, err := json.Marshal(policy.DropFields)
	if err != nil {
		return FilterPolicyRow{}, fmt.Errorf("persistence: marshal drop_fields for %q: %w", policy.Name, err)
	}
	return FilterPolicyRow{
		Name:       policy.Name,
		Match:      datatypes.JSON(match),
		MCP:        policy.MCP,
		Tool:       policy.Tool,
		DropFields: datatypes.JSON(dropFields),
	}, nil
}

func filterPolicyFrom(row FilterPolicyRow) (router.FilterPolicy, error) {
	match, err := unmarshalClaimMatcher(row.Match)
	if err != nil {
		return router.FilterPolicy{}, fmt.Errorf("persistence: filter policy %q: %w", row.Name, err)
	}

	var dropFields []string
	if len(row.DropFields) > 0 {
		if err := json.Unmarshal(row.DropFields, &dropFields); err != nil {
			return router.FilterPolicy{}, fmt.Errorf("persistence: unmarshal drop_fields for %q: %w", row.Name, err)
		}
	}

	return router.FilterPolicy{
		Name:       row.Name,
		Match:      match,
		MCP:        row.MCP,
		Tool:       row.Tool,
		DropFields: dropFields,
	}, nil
}
