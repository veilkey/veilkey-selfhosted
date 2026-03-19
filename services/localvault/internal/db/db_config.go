package db

import (
	"fmt"

	"gorm.io/gorm/clause"
)

func (d *DB) SaveConfig(key, value string) error {
	cfg := Config{
		Key:    key,
		Value:  value,
		Scope:  RefScopeLocal,
		Status: RefStatusActive,
	}
	return d.conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "scope", "status", "updated_at"}),
	}).Create(&cfg).Error
}

func (d *DB) SaveConfigs(configs map[string]string) error {
	cfgs := make([]Config, 0, len(configs))
	for k, v := range configs {
		cfgs = append(cfgs, Config{
			Key:    k,
			Value:  v,
			Scope:  RefScopeLocal,
			Status: RefStatusActive,
		})
	}
	if len(cfgs) == 0 {
		return nil
	}
	return d.conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "scope", "status", "updated_at"}),
	}).Create(&cfgs).Error
}

func (d *DB) GetConfig(key string) (*Config, error) {
	var c Config
	if err := d.conn.Where("key = ?", key).First(&c).Error; err != nil {
		return nil, fmt.Errorf("config %s not found", key)
	}
	return &c, nil
}

func (d *DB) ListConfigs() ([]Config, error) {
	var configs []Config
	err := d.conn.Order("key").Find(&configs).Error
	return configs, err
}

func (d *DB) DeleteConfig(key string) error {
	result := d.conn.Where("key = ?", key).Delete(&Config{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("config %s not found", key)
	}
	return nil
}

func (d *DB) CountConfigs() (int, error) {
	var count int64
	err := d.conn.Model(&Config{}).Count(&count).Error
	return int(count), err
}

func (d *DB) UpdateConfigLifecycle(key string, scope RefScope, status RefStatus) error {
	var updates map[string]interface{}
	if scope == "" {
		updates = map[string]interface{}{"status": status}
	} else {
		updates = map[string]interface{}{"scope": scope, "status": status}
	}

	result := d.conn.Model(&Config{}).Where("key = ?", key).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("config %s not found", key)
	}
	return nil
}
