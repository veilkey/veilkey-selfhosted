package db

import (
	"fmt"

	"github.com/veilkey/veilkey-go-package/dbutil"
	"gorm.io/gorm"
)

type DB struct {
	conn *gorm.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := dbutil.OpenGORM(dbPath)
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (d *DB) migrate() error {
	if err := d.conn.AutoMigrate(
		&EncryptionKey{},
		&TokenRef{},
		&NodeInfo{},
		&Child{},
		&VaultInventory{},
		&SecretCatalog{},
		&Binding{},
		&AuditEvent{},
		&KeyRegistryEntry{},
		&Secret{},
		&Agent{},
		&GlobalFunction{},
		&SecretInputChallenge{},
		&EmailOTPChallenge{},
		&ApprovalTokenChallenge{},
		&AdminAuthConfig{},
		&AdminSession{},
		&UIConfig{},
		&Config{},
		&BulkApplyTemplate{},
		&BulkApplyRun{},
		&RegistrationToken{},
		&AdminPasskey{},
		&SSHKey{},
	); err != nil {
		return err
	}

	if err := d.NormalizeTokenRefStorage(); err != nil {
		return err
	}
	if err := d.BackfillAgentCapabilities(); err != nil {
		return err
	}
	if err := d.EnsureTokenRefCanonicalUniqueness(); err != nil {
		return err
	}
	if _, err := d.BackfillVaultInventoryFromAgents(); err != nil {
		return err
	}
	if _, err := d.BackfillSecretCatalogFromTrackedRefs(); err != nil {
		return err
	}
	return d.PromoteOperationalTempRefs(nil)
}

// dbFirst fetches the first record matching query. Returns notFound error if no record exists.
func dbFirst[T any](d *DB, notFound, query string, args ...any) (*T, error) {
	var out T
	conds := append([]any{query}, args...)
	if err := d.conn.First(&out, conds...).Error; err != nil {
		return nil, fmt.Errorf("%s", notFound)
	}
	return &out, nil
}

// dbDeleteWhere deletes records matching query. Returns notFound error if no rows affected.
func dbDeleteWhere[T any](d *DB, notFound, query string, args ...any) error {
	conds := append([]any{query}, args...)
	result := d.conn.Delete(new(T), conds...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%s", notFound)
	}
	return nil
}

func (d *DB) Close() error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (d *DB) Ping() error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
