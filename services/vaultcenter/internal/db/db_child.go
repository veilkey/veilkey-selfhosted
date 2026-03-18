package db

import (
	"fmt"
	"time"
)

func (d *DB) RegisterChild(child *Child) error {
	return d.conn.Save(child).Error
}

func (d *DB) GetChild(nodeID string) (*Child, error) {
	return dbFirst[Child](d, "child "+nodeID+" not found", "node_id = ?", nodeID)
}
func (d *DB) ListChildren() ([]Child, error) {
	var children []Child
	err := d.conn.Order("created_at").Find(&children).Error
	return children, err
}

func (d *DB) UpdateChildURL(nodeID, url string) error {
	now := time.Now()
	result := d.conn.Model(&Child{}).Where("node_id = ?", nodeID).
		Select("URL", "LastSeen").
		Updates(&Child{URL: url, LastSeen: &now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("child %s not found", nodeID)
	}
	return nil
}

func (d *DB) UpdateChildDEK(nodeID string, encryptedDEK, nonce []byte, version int) error {
	result := d.conn.Model(&Child{}).Where("node_id = ?", nodeID).
		Select("EncryptedDEK", "Nonce", "Version").
		Updates(&Child{EncryptedDEK: encryptedDEK, Nonce: nonce, Version: version})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("child %s not found", nodeID)
	}
	return nil
}

func (d *DB) UpdateChildLastSeen(nodeID string) error {
	now := time.Now()
	return d.conn.Model(&Child{}).Where("node_id = ?", nodeID).
		Update("last_seen", now).Error
}

func (d *DB) DeleteChild(nodeID string) error {
	result := d.conn.Delete(&Child{}, "node_id = ?", nodeID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("child %s not found", nodeID)
	}
	d.conn.Delete(&KeyRegistryEntry{}, "node_id = ?", nodeID)
	return nil
}

func (d *DB) CountChildren() (int, error) {
	var count int64
	err := d.conn.Model(&Child{}).Count(&count).Error
	return int(count), err
}
