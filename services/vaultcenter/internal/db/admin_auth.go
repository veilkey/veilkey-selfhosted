package db

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (d *DB) SetAdminPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	cfg, err := d.GetOrCreateAdminAuthConfig()
	if err != nil {
		return err
	}
	cfg.PasswordHash = string(hash)
	return d.SaveAdminAuthConfig(cfg)
}

func (d *DB) HasAdminPassword() bool {
	cfg, err := d.GetAdminAuthConfig()
	return err == nil && cfg.PasswordHash != ""
}

func (d *DB) VerifyAdminPassword(password string) bool {
	cfg, err := d.GetAdminAuthConfig()
	if err != nil || cfg.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(password)) == nil
}

const defaultAdminConfigID = "default"

func (d *DB) GetAdminAuthConfig() (*AdminAuthConfig, error) {
	var cfg AdminAuthConfig
	if err := d.conn.First(&cfg, "config_id = ?", defaultAdminConfigID).Error; err != nil {
		return nil, fmt.Errorf("admin auth config not found")
	}
	return &cfg, nil
}

func (d *DB) SaveAdminAuthConfig(cfg *AdminAuthConfig) error {
	if cfg == nil {
		return fmt.Errorf("admin auth config is required")
	}
	if cfg.ConfigID == "" {
		cfg.ConfigID = defaultAdminConfigID
	}
	return d.conn.Save(cfg).Error
}

func (d *DB) GetOrCreateAdminAuthConfig() (*AdminAuthConfig, error) {
	cfg, err := d.GetAdminAuthConfig()
	if err == nil {
		return cfg, nil
	}
	cfg = &AdminAuthConfig{ConfigID: defaultAdminConfigID}
	if err := d.SaveAdminAuthConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (d *DB) SaveAdminSession(session *AdminSession) error {
	if session == nil {
		return fmt.Errorf("admin session is required")
	}
	if session.SessionID == "" || session.TokenHash == "" {
		return fmt.Errorf("session_id and token_hash are required")
	}
	return d.conn.Save(session).Error
}

func (d *DB) GetAdminSessionByTokenHash(tokenHash string) (*AdminSession, error) {
	var session AdminSession
	if err := d.conn.
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		First(&session).Error; err != nil {
		return nil, fmt.Errorf("admin session not found")
	}
	return &session, nil
}

func (d *DB) TouchAdminSession(sessionID string, lastSeenAt, idleExpiresAt time.Time) error {
	return d.conn.Model(&AdminSession{}).
		Where("session_id = ? AND revoked_at IS NULL", sessionID).
		Select("LastSeenAt", "IdleExpiresAt").
		Updates(&AdminSession{LastSeenAt: lastSeenAt.UTC(), IdleExpiresAt: idleExpiresAt.UTC()}).Error
}

func (d *DB) UpdateAdminSessionRevealUntil(sessionID string, revealUntil *time.Time) error {
	return d.conn.Model(&AdminSession{}).
		Where("session_id = ? AND revoked_at IS NULL", sessionID).
		Update("reveal_until", revealUntil).Error
}

func (d *DB) RevokeAdminSession(sessionID string, revokedAt time.Time) error {
	return d.conn.Model(&AdminSession{}).
		Where("session_id = ? AND revoked_at IS NULL", sessionID).
		Update("revoked_at", revokedAt.UTC()).Error
}
