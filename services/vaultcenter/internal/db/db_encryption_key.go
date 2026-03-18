package db

import "fmt"

func (d *DB) GetActiveKey() (*EncryptionKey, error) {
	var key EncryptionKey
	err := d.conn.Where("status = ?", RefStatusActive).Order("version DESC").First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (d *DB) GetKeyByVersion(version int) (*EncryptionKey, error) {
	var key EncryptionKey
	err := d.conn.First(&key, "version = ?", version).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (d *DB) CreateKey(encryptedDEK, nonce []byte) (int, error) {
	key := EncryptionKey{
		EncryptedDEK: encryptedDEK,
		Nonce:        nonce,
	}
	if err := d.conn.Create(&key).Error; err != nil {
		return 0, err
	}
	return key.Version, nil
}

func (d *DB) GetAllKeys() ([]EncryptionKey, error) {
	var keys []EncryptionKey
	err := d.conn.Order("version DESC").Find(&keys).Error
	return keys, err
}

func (d *DB) UpdateKeyEncryption(version int, encryptedDEK, nonce []byte) error {
	result := d.conn.Model(&EncryptionKey{}).Where("version = ?", version).
		Updates(map[string]interface{}{
			"encrypted_dek": encryptedDEK,
			"nonce":         nonce,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("key version %d not found", version)
	}
	return nil
}
