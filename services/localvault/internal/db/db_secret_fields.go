package db

import (
	"fmt"

	"gorm.io/gorm/clause"
)

func coalesceString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (d *DB) SaveSecretFields(secretName string, fields []SecretField) error {
	if secretName == "" {
		return fmt.Errorf("secret name is required")
	}
	for i := range fields {
		fields[i].SecretName = secretName
		if fields[i].FieldRole == "" {
			fields[i].FieldRole = fields[i].FieldType
		}
		if fields[i].DisplayName == "" {
			fields[i].DisplayName = fields[i].FieldKey
		}
	}
	return d.conn.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "secret_name"}, {Name: "field_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"field_type", "field_role", "display_name",
			"masked_by_default", "required", "sort_order",
			"ciphertext", "nonce", "updated_at",
		}),
	}).Create(&fields).Error
}

func (d *DB) ListSecretFields(secretName string) ([]SecretField, error) {
	var fields []SecretField
	err := d.conn.Where("secret_name = ?", secretName).Order("field_key").Find(&fields).Error
	return fields, err
}

func (d *DB) GetSecretField(secretName, fieldKey string) (*SecretField, error) {
	var field SecretField
	if err := d.conn.Where("secret_name = ? AND field_key = ?", secretName, fieldKey).First(&field).Error; err != nil {
		return nil, fmt.Errorf("secret field %s.%s not found", secretName, fieldKey)
	}
	return &field, nil
}

func (d *DB) DeleteSecretField(secretName, fieldKey string) error {
	result := d.conn.Where("secret_name = ? AND field_key = ?", secretName, fieldKey).Delete(&SecretField{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("secret field %s.%s not found", secretName, fieldKey)
	}
	return nil
}
