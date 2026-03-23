package db

import "fmt"

func (d *DB) HasNodeInfo() bool {
	var count int64
	d.conn.Model(&NodeInfo{}).Count(&count)
	return count > 0
}

func (d *DB) GetNodeInfo() (*NodeInfo, error) {
	var info NodeInfo
	if err := d.conn.First(&info).Error; err != nil {
		return nil, err
	}
	return &info, nil
}

func (d *DB) SaveNodeInfo(info *NodeInfo) error {
	return d.conn.Create(info).Error
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

func (d *DB) UpdateAgentSecret(encSecret, encNonce []byte) error {
	result := d.conn.Model(&NodeInfo{}).Where("1 = 1").
		Select("AgentSecret", "AgentSecretNonce").
		Updates(&NodeInfo{AgentSecret: encSecret, AgentSecretNonce: encNonce})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no node_info to update")
	}
	return nil
}

func (d *DB) UpdateNodeVersion(version int) error {
	result := d.conn.Model(&NodeInfo{}).Where("1 = 1").
		Update("version", version)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no node_info to update")
	}
	return nil
}
