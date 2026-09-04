package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/atsokha/mcplake/cache"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MCPRegistrationRepo persists cache.MCPRegistration records (minus the live
// cache.MCPClient, which is never durable state) via MCPRegistrationRow.
type MCPRegistrationRepo struct {
	db *gorm.DB
}

// NewMCPRegistrationRepo returns a repository backed by db. db is expected to
// already have run AutoMigrate (see Open).
func NewMCPRegistrationRepo(db *gorm.DB) *MCPRegistrationRepo {
	return &MCPRegistrationRepo{db: db}
}

// Upsert inserts reg, or updates the existing row with the same Name in
// place. reg.Client is ignored — it is never part of the persisted shape.
func (r *MCPRegistrationRepo) Upsert(ctx context.Context, reg cache.MCPRegistration) error {
	row, err := mcpRegistrationRowFrom(reg)
	if err != nil {
		return err
	}

	err = r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"transport", "connect", "tools", "status", "updated_at"}),
		}).
		Create(&row).Error
	if err != nil {
		return fmt.Errorf("persistence: upsert mcp registration %q: %w", reg.Name, err)
	}
	return nil
}

// Get returns the registration stored under name, if any. found is false
// (with a nil error) when no such registration exists.
func (r *MCPRegistrationRepo) Get(ctx context.Context, name string) (reg cache.MCPRegistration, found bool, err error) {
	var row MCPRegistrationRow
	err = r.db.WithContext(ctx).First(&row, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cache.MCPRegistration{}, false, nil
	}
	if err != nil {
		return cache.MCPRegistration{}, false, fmt.Errorf("persistence: get mcp registration %q: %w", name, err)
	}

	reg, err = mcpRegistrationFrom(row)
	if err != nil {
		return cache.MCPRegistration{}, false, err
	}
	return reg, true, nil
}

// List returns every stored registration.
func (r *MCPRegistrationRepo) List(ctx context.Context) ([]cache.MCPRegistration, error) {
	var rows []MCPRegistrationRow
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("persistence: list mcp registrations: %w", err)
	}

	regs := make([]cache.MCPRegistration, 0, len(rows))
	for _, row := range rows {
		reg, err := mcpRegistrationFrom(row)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}
	return regs, nil
}

// Delete permanently removes the registration stored under name. It uses
// Unscoped to hard-delete rather than relying on gorm.Model's default soft
// delete: a soft-deleted row would keep occupying the unique "name" index,
// which would break re-registering the same name later (ADR-0003's
// make-before-break re-registration).
func (r *MCPRegistrationRepo) Delete(ctx context.Context, name string) error {
	err := r.db.WithContext(ctx).Unscoped().Where("name = ?", name).Delete(&MCPRegistrationRow{}).Error
	if err != nil {
		return fmt.Errorf("persistence: delete mcp registration %q: %w", name, err)
	}
	return nil
}

func mcpRegistrationRowFrom(reg cache.MCPRegistration) (MCPRegistrationRow, error) {
	connect, err := json.Marshal(reg.Connect)
	if err != nil {
		return MCPRegistrationRow{}, fmt.Errorf("persistence: marshal connect config for %q: %w", reg.Name, err)
	}
	tools, err := json.Marshal(reg.Tools)
	if err != nil {
		return MCPRegistrationRow{}, fmt.Errorf("persistence: marshal tools for %q: %w", reg.Name, err)
	}
	return MCPRegistrationRow{
		Name:      reg.Name,
		Transport: reg.Transport,
		Connect:   datatypes.JSON(connect),
		Tools:     datatypes.JSON(tools),
		Status:    reg.Status,
	}, nil
}

func mcpRegistrationFrom(row MCPRegistrationRow) (cache.MCPRegistration, error) {
	var connect cache.ConnectConfig
	if len(row.Connect) > 0 {
		if err := json.Unmarshal(row.Connect, &connect); err != nil {
			return cache.MCPRegistration{}, fmt.Errorf("persistence: unmarshal connect config for %q: %w", row.Name, err)
		}
	}

	var tools map[string]cache.ToolSchema
	if len(row.Tools) > 0 {
		if err := json.Unmarshal(row.Tools, &tools); err != nil {
			return cache.MCPRegistration{}, fmt.Errorf("persistence: unmarshal tools for %q: %w", row.Name, err)
		}
	}

	return cache.MCPRegistration{
		Name:      row.Name,
		Transport: row.Transport,
		Connect:   connect,
		Status:    row.Status,
		Tools:     tools,
	}, nil
}
