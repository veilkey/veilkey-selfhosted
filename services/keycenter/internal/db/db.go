package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	conn *gorm.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := gorm.Open(sqlite.Open(dbPath+"?_journal_mode=wal&_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
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
		&InstallSession{},
		&InstallCustodyChallenge{},
		&SecretInputChallenge{},
		&EmailOTPChallenge{},
		&ApprovalTokenChallenge{},
		&AdminAuthConfig{},
		&AdminSession{},
		&UIConfig{},
		&InstallRun{},
		&Config{},
		&BulkApplyTemplate{},
		&BulkApplyRun{},
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
