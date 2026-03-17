package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"

	_ "github.com/mattn/go-sqlite3" // replaced by go-sqlcipher/v4 via go.mod replace directive
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	conn *gorm.DB
}

func New(dbPath string) (*DB, error) {
	dsn := dbPath + "?_journal_mode=wal&_busy_timeout=5000"

	// SQLCipher: 환경변수로 DB 암호화 키가 설정된 경우 DSN에 _pragma_key 추가
	if key := os.Getenv("VEILKEY_DB_KEY"); key != "" {
		dsn += "&_pragma_key=" + url.QueryEscape(key)
	}

	// go-sqlcipher/v4가 등록한 "sqlite3" 드라이버로 직접 연결 후 GORM에 전달
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// SQLCipher 키가 설정된 경우, 드라이버 지원 여부와 DB 접근 가능 여부를 함께 검증
	if os.Getenv("VEILKEY_DB_KEY") != "" {
		version, verErr := sqlCipherVersion(sqlDB)
		if verErr != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("sqlcipher 지원 확인 실패: %w", verErr)
		}
		if version == "" {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("VEILKEY_DB_KEY가 설정되었으나 바이너리가 SQLCipher 없이 빌드됨")
		}
		if _, verErr = sqlDB.Exec("SELECT count(*) FROM sqlite_master"); verErr != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("sqlcipher DB 키 검증 실패: %w", verErr)
		}
	}

	conn, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

// sqlCipherVersion checks if the underlying driver supports SQLCipher.
func sqlCipherVersion(conn *sql.DB) (string, error) {
	var version sql.NullString
	err := conn.QueryRow("PRAGMA cipher_version").Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return version.String, nil
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
