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

// AccessPolicyRepo persists router.AccessPolicy records via AccessPolicyRow.
type AccessPolicyRepo struct {
	db *gorm.DB
}

// NewAccessPolicyRepo returns a repository backed by db. db is expected to
// already have run AutoMigrate (see Open).
func NewAccessPolicyRepo(db *gorm.DB) *AccessPolicyRepo {
	return &AccessPolicyRepo{db: db}
}

// Upsert inserts policy, or updates the existing row with the same Name in
// place. A Match rule that fails to compile (router.NewClaimRule) is
// rejected here — the policy is not persisted.
func (r *AccessPolicyRepo) Upsert(ctx context.Context, policy router.AccessPolicy) error {
	if err := validateClaimMatcher(policy.Match); err != nil {
		return fmt.Errorf("persistence: upsert access policy %q: %w", policy.Name, err)
	}

	row, err := accessPolicyRowFrom(policy)
	if err != nil {
		return err
	}

	err = r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"match", "grants", enabledColumn, "updated_at"}),
		}).
		Create(&row).Error
	if err != nil {
		return fmt.Errorf("persistence: upsert access policy %q: %w", policy.Name, err)
	}
	return nil
}

// Get returns the policy stored under name, if any, with its ClaimMatcher
// already recompiled and directly usable. found is false (with a nil error)
// when no such policy exists.
func (r *AccessPolicyRepo) Get(ctx context.Context, name string) (policy router.AccessPolicy, found bool, err error) {
	var row AccessPolicyRow
	err = r.db.WithContext(ctx).First(&row, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return router.AccessPolicy{}, false, nil
	}
	if err != nil {
		return router.AccessPolicy{}, false, fmt.Errorf("persistence: get access policy %q: %w", name, err)
	}

	policy, err = accessPolicyFrom(row)
	if err != nil {
		return router.AccessPolicy{}, false, err
	}
	return policy, true, nil
}

// List returns every stored access policy.
func (r *AccessPolicyRepo) List(ctx context.Context) ([]router.AccessPolicy, error) {
	var rows []AccessPolicyRow
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("persistence: list access policies: %w", err)
	}

	policies := make([]router.AccessPolicy, 0, len(rows))
	for _, row := range rows {
		policy, err := accessPolicyFrom(row)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, nil
}

// Delete permanently removes the policy stored under name (hard delete, see
// MCPRegistrationRepo.Delete for why: a soft-deleted row would keep
// occupying the unique "name" index).
func (r *AccessPolicyRepo) Delete(ctx context.Context, name string) error {
	err := r.db.WithContext(ctx).Unscoped().Where("name = ?", name).Delete(&AccessPolicyRow{}).Error
	if err != nil {
		return fmt.Errorf("persistence: delete access policy %q: %w", name, err)
	}
	return nil
}

func accessPolicyRowFrom(policy router.AccessPolicy) (AccessPolicyRow, error) {
	match, err := marshalClaimMatcher(policy.Match)
	if err != nil {
		return AccessPolicyRow{}, err
	}
	grants, err := json.Marshal(policy.Grants)
	if err != nil {
		return AccessPolicyRow{}, fmt.Errorf("persistence: marshal grants for %q: %w", policy.Name, err)
	}
	return AccessPolicyRow{
		Name:    policy.Name,
		Match:   datatypes.JSON(match),
		Grants:  datatypes.JSON(grants),
		Enabled: &policy.Enabled,
	}, nil
}

func accessPolicyFrom(row AccessPolicyRow) (router.AccessPolicy, error) {
	match, err := unmarshalClaimMatcher(row.Match)
	if err != nil {
		return router.AccessPolicy{}, fmt.Errorf("persistence: access policy %q: %w", row.Name, err)
	}

	var grants []router.Grant
	if len(row.Grants) > 0 {
		if err := json.Unmarshal(row.Grants, &grants); err != nil {
			return router.AccessPolicy{}, fmt.Errorf("persistence: unmarshal grants for %q: %w", row.Name, err)
		}
	}

	return router.AccessPolicy{
		Name:    row.Name,
		Match:   match,
		Grants:  grants,
		Enabled: enabledValue(row.Enabled),
	}, nil
}
