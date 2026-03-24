package db

import (
	"fmt"
	"time"
)

func (d *DB) SaveRegistrationToken(token *RegistrationToken) error {
	return d.conn.Create(token).Error
}

func (d *DB) GetRegistrationToken(tokenID string) (*RegistrationToken, error) {
	var token RegistrationToken
	if err := d.conn.Where("token_id = ?", tokenID).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func (d *DB) ConsumeRegistrationToken(tokenID, usedByNode string) error {
	now := time.Now().UTC()
	result := d.conn.Model(&RegistrationToken{}).
		Where("token_id = ? AND status = ? AND expires_at > ?", tokenID, "active", now).
		Updates(map[string]interface{}{
			"status":       "used",
			"used_by_node": usedByNode,
			"used_at":      &now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("token not found, expired, or already used")
	}
	return nil
}

func (d *DB) ListRegistrationTokens(limit, offset int) ([]RegistrationToken, int64, error) {
	var tokens []RegistrationToken
	var total int64
	d.conn.Model(&RegistrationToken{}).Count(&total)
	if err := d.conn.Order("created_at DESC").Limit(limit).Offset(offset).Find(&tokens).Error; err != nil {
		return nil, 0, err
	}
	return tokens, total, nil
}

func (d *DB) RevokeRegistrationToken(tokenID string) error {
	return d.conn.Model(&RegistrationToken{}).
		Where("token_id = ? AND status = ?", tokenID, "active").
		Update("status", "revoked").Error
}

func (d *DB) DeleteExpiredRegistrationTokens() (int64, error) {
	result := d.conn.Model(&RegistrationToken{}).Where("status = ? AND expires_at < ?", "active", time.Now().UTC()).
		Updates(map[string]interface{}{"status": "expired"})
	return result.RowsAffected, result.Error
}

func (d *DB) ValidateRegistrationToken(tokenID string) (*RegistrationToken, error) {
	var token RegistrationToken
	if err := d.conn.Where("token_id = ? AND status = ? AND expires_at > ?", tokenID, "active", time.Now().UTC()).
		First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}
