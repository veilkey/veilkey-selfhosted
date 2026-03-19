package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func normalizeFunctionScope(scope string) (string, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	switch scope {
	case "GLOBAL", "VAULT", "LOCAL", "TEST":
		return scope, nil
	default:
		return "", fmt.Errorf("invalid function scope: %s", scope)
	}
}

func (d *DB) SaveFunction(fn *Function) error {
	if fn == nil {
		return fmt.Errorf("function is required")
	}
	if fn.Name == "" {
		return fmt.Errorf("function name is required")
	}
	if fn.Scope == "" {
		return fmt.Errorf("function scope is required")
	}
	normalizedScope, err := normalizeFunctionScope(fn.Scope)
	if err != nil {
		return err
	}
	fn.Scope = normalizedScope
	if fn.VaultHash == "" {
		return fmt.Errorf("vault_hash is required")
	}
	if fn.FunctionHash == "" {
		return fmt.Errorf("function_hash is required")
	}
	if fn.Command == "" {
		return fmt.Errorf("command is required")
	}
	if fn.VarsJSON == "" {
		fn.VarsJSON = "{}"
	}
	fn.TagsJSON = coalesceString(fn.TagsJSON, "[]")
	fn.Provenance = coalesceString(fn.Provenance, "local")

	return d.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"scope", "vault_hash", "function_hash", "category", "command",
				"vars_json", "description", "tags_json", "provenance",
				"last_tested_at", "last_run_at", "updated_at",
			}),
		}).Create(fn).Error; err != nil {
			return err
		}
		return insertFunctionLogTx(tx, fn.FunctionHash, "save", "ok", map[string]string{
			"name":       fn.Name,
			"scope":      fn.Scope,
			"vault_hash": fn.VaultHash,
		})
	})
}

func (d *DB) GetFunction(name string) (*Function, error) {
	var fn Function
	if err := d.conn.Where("name = ?", name).First(&fn).Error; err != nil {
		return nil, fmt.Errorf("function %s not found", name)
	}
	return &fn, nil
}

func (d *DB) ListFunctions() ([]Function, error) {
	return d.ListFunctionsByScope("")
}

func (d *DB) ListFunctionsByScope(scope string) ([]Function, error) {
	if scope != "" {
		normalizedScope, err := normalizeFunctionScope(scope)
		if err != nil {
			return nil, err
		}
		scope = normalizedScope
	}

	var out []Function
	q := d.conn.Order("name")
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}
	err := q.Find(&out).Error
	return out, err
}

func (d *DB) DeleteFunction(name string) error {
	fn, err := d.GetFunction(name)
	if err != nil {
		return err
	}

	return d.conn.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("name = ?", name).Delete(&Function{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("function %s not found", name)
		}
		return insertFunctionLogTx(tx, fn.FunctionHash, "delete", "ok", map[string]string{
			"name":       fn.Name,
			"scope":      fn.Scope,
			"vault_hash": fn.VaultHash,
		})
	})
}

func (d *DB) CleanupExpiredTestFunctions(now time.Time) (int, error) {
	var testFns []Function
	if err := d.conn.Where("scope = ?", "TEST").Find(&testFns).Error; err != nil {
		return 0, err
	}

	var expired []Function
	cutoff := now.Add(-1 * time.Hour)
	for _, fn := range testFns {
		if !fn.CreatedAt.After(cutoff) {
			expired = append(expired, fn)
		}
	}

	if len(expired) == 0 {
		return 0, nil
	}

	err := d.conn.Transaction(func(tx *gorm.DB) error {
		for _, fn := range expired {
			if err := tx.Where("name = ?", fn.Name).Delete(&Function{}).Error; err != nil {
				return err
			}
			if err := insertFunctionLogTx(tx, fn.FunctionHash, "cleanup", "deleted", map[string]string{
				"name":       fn.Name,
				"scope":      fn.Scope,
				"vault_hash": fn.VaultHash,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return len(expired), err
}

func (d *DB) CountFunctions() (int, error) {
	var count int64
	err := d.conn.Model(&Function{}).Count(&count).Error
	return int(count), err
}

func (d *DB) ListFunctionLogs() ([]FunctionLog, error) {
	var out []FunctionLog
	err := d.conn.Order("id").Find(&out).Error
	return out, err
}

func insertFunctionLogTx(tx *gorm.DB, functionHash, action, status string, detail interface{}) error {
	payload := "{}"
	if detail != nil {
		raw, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		payload = string(raw)
	}
	return tx.Create(&FunctionLog{
		FunctionHash: functionHash,
		Action:       action,
		Status:       status,
		DetailJSON:   payload,
	}).Error
}
