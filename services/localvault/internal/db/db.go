package db

import (
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3" // replaced by go-sqlcipher/v4 via go.mod replace directive
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	conn *gorm.DB
}

func New(dbPath string) (*DB, error) {
	dsn := dbPath + "?_journal_mode=wal&_busy_timeout=5000"

	// SQLCipher: 환경변수로 DB 암호화 키가 설정된 경우 DSN에 _pragma_key 추가
	if key := os.Getenv("VEILKEY_DB_KEY"); key != "" {
		dsn += "&_pragma_key=" + url.QueryEscape(key)
	}

	// go-sqlcipher/v4 registers as "sqlite3"; open raw connection then hand to GORM
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
	if err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS migrations (
		filename   TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		filename := entry.Name()

		var count int64
		d.conn.Raw(`SELECT COUNT(*) FROM migrations WHERE filename = ?`, filename).Scan(&count)
		if count > 0 {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}
		if err := d.conn.Exec(string(sqlBytes)).Error; err != nil {
			return fmt.Errorf("migration %s failed: %w", filename, err)
		}
		if err := d.conn.Exec(`INSERT INTO migrations (filename) VALUES (?)`, filename).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}
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
