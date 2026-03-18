package db

import "fmt"

func (d *DB) HasNodeInfo() bool {
	var count int64
	d.conn.Model(&NodeInfo{}).Count(&count)
	return count > 0
}

func (d *DB) GetNodeInfo() (*NodeInfo, error) {
	var info NodeInfo
	err := d.conn.First(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (d *DB) SaveNodeInfo(info *NodeInfo) error {
	return d.conn.Create(info).Error
}

func (d *DB) SetParentURL(parentURL string) (int64, error) {
	result := d.conn.Model(&NodeInfo{}).Where("1 = 1").Update("parent_url", parentURL)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, fmt.Errorf("no node_info to update")
	}
	return result.RowsAffected, nil
}

func (d *DB) UpdateNodeDEK(dek, nonce []byte, version int) error {
	result := d.conn.Model(&NodeInfo{}).Where("1 = 1").
		Select("DEK", "DEKNonce", "Version").
		Updates(&NodeInfo{DEK: dek, DEKNonce: nonce, Version: version})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no node_info to update")
	}
	return nil
}
