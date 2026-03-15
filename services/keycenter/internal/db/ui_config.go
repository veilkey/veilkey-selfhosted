package db

import "fmt"

const defaultUIConfigID = "default"

func (d *DB) GetUIConfig() (*UIConfig, error) {
	var cfg UIConfig
	if err := d.conn.First(&cfg, "config_id = ?", defaultUIConfigID).Error; err != nil {
		return nil, fmt.Errorf("ui config not found")
	}
	return &cfg, nil
}

func (d *DB) SaveUIConfig(cfg *UIConfig) error {
	if cfg == nil {
		return fmt.Errorf("ui config is required")
	}
	if cfg.ConfigID == "" {
		cfg.ConfigID = defaultUIConfigID
	}
	if cfg.Locale == "" {
		cfg.Locale = "ko"
	}
	if cfg.ReleaseChannel == "" {
		cfg.ReleaseChannel = "stable"
	}
	return d.conn.Save(cfg).Error
}

func (d *DB) GetOrCreateUIConfig() (*UIConfig, error) {
	cfg, err := d.GetUIConfig()
	if err == nil {
		return cfg, nil
	}
	cfg = &UIConfig{ConfigID: defaultUIConfigID, Locale: "ko", ReleaseChannel: "stable"}
	if err := d.SaveUIConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
