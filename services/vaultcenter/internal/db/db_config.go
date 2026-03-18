package db

import (
	"gorm.io/gorm"
)

func (d *DB) SaveConfig(key, value string) error {
	entry := &Config{
		Key:    key,
		Value:  value,
		Scope:  RefScopeLocal,
		Status: RefStatusActive,
	}
	return d.conn.Save(entry).Error
}

func (d *DB) SaveConfigs(configs map[string]string) error {
	return d.conn.Transaction(func(tx *gorm.DB) error {
		for key, value := range configs {
			entry := &Config{
				Key:    key,
				Value:  value,
				Scope:  RefScopeLocal,
				Status: RefStatusActive,
			}
			if err := tx.Save(entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *DB) GetConfig(key string) (*Config, error) {
	return dbFirst[Config](d, "config "+key+" not found", "key = ?", key)
}
func (d *DB) ListConfigs() ([]Config, error) {
	var entries []Config
	if err := d.conn.Order("key asc").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (d *DB) DeleteConfig(key string) error {
	return dbDeleteWhere[Config](d, "config "+key+" not found", "key = ?", key)
}
func (d *DB) CountConfigs() (int, error) {
	var count int64
	if err := d.conn.Model(&Config{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}
