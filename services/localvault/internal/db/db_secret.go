package db

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

func (d *DB) SaveSecret(secret *Secret) error {
	if secret.Status == "" {
		secret.Status = RefStatusActive
	}
	if secret.Scope == "" {
		secret.Scope = RefScopeLocal
	}
	return d.conn.Save(secret).Error
}

func (d *DB) GetSecretByName(name string) (*Secret, error) {
	var s Secret
	if err := d.conn.Where("name = ?", name).First(&s).Error; err != nil {
		return nil, fmt.Errorf("secret %s not found", name)
	}
	return &s, nil
}

func (d *DB) GetSecretByRef(refHash string) (*Secret, error) {
	var s Secret
	if err := d.conn.Where("ref = ?", refHash).First(&s).Error; err != nil {
		return nil, fmt.Errorf("secret ref %s not found", refHash)
	}
	return &s, nil
}

func (d *DB) ListSecrets() ([]Secret, error) {
	var secrets []Secret
	err := d.conn.Order("name").Find(&secrets).Error
	return secrets, err
}

func (d *DB) DeleteSecret(name string) error {
	result := d.conn.Where("name = ?", name).Delete(&Secret{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("secret %s not found", name)
	}
	return nil
}

func (d *DB) UpdateSecretStatus(refHash string, status RefStatus) error {
	result := d.conn.Model(&Secret{}).Where("ref = ?", refHash).
		Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) UpdateSecretLifecycle(refHash string, scope RefScope, status RefStatus) error {
	result := d.conn.Model(&Secret{}).Where("ref = ?", refHash).
		Updates(map[string]interface{}{"scope": scope, "status": status})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) MarkSecretRevealed(refHash string, revealedAt time.Time) error {
	t := revealedAt.UTC()
	result := d.conn.Model(&Secret{}).Where("ref = ?", refHash).
		Update("last_revealed_at", &t)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) MarkSecretRotated(refHash string, rotatedAt time.Time) error {
	t := rotatedAt.UTC()
	result := d.conn.Model(&Secret{}).Where("ref = ?", refHash).
		Update("last_rotated_at", &t)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) CountSecrets() (int, error) {
	var count int64
	err := d.conn.Model(&Secret{}).Count(&count).Error
	return int(count), err
}

func (d *DB) ReencryptAllSecrets(
	decryptFn func(ciphertext, nonce []byte) ([]byte, error),
	encryptFn func(plaintext []byte) (ciphertext, nonce []byte, err error),
	newVersion int,
) (int, error) {
	secrets, err := d.ListSecrets()
	if err != nil {
		return 0, err
	}

	count := 0
	err = d.conn.Transaction(func(tx *gorm.DB) error {
		for i := range secrets {
			s := &secrets[i]
			plaintext, err := decryptFn(s.Ciphertext, s.Nonce)
			if err != nil {
				return fmt.Errorf("decrypt secret %s: %w", s.Name, err)
			}
			newCiphertext, newNonce, err := encryptFn(plaintext)
			if err != nil {
				return fmt.Errorf("encrypt secret %s: %w", s.Name, err)
			}
			now := time.Now().UTC()
			if err := tx.Model(s).Select("Ciphertext", "Nonce", "Version", "LastRotatedAt").Updates(&Secret{
				Ciphertext:    newCiphertext,
				Nonce:         newNonce,
				Version:       newVersion,
				LastRotatedAt: &now,
			}).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (d *DB) ReencryptMixedSecrets(
	decryptOldFn func(ciphertext, nonce []byte) ([]byte, error),
	decryptCurrentFn func(ciphertext, nonce []byte) ([]byte, error),
	encryptFn func(plaintext []byte) (ciphertext, nonce []byte, err error),
	newVersion int,
) (int, int, error) {
	secrets, err := d.ListSecrets()
	if err != nil {
		return 0, 0, err
	}

	updated := 0
	skipped := 0

	err = d.conn.Transaction(func(tx *gorm.DB) error {
		for i := range secrets {
			s := &secrets[i]
			if s.Version == newVersion && decryptCurrentFn != nil {
				if _, err := decryptCurrentFn(s.Ciphertext, s.Nonce); err == nil {
					skipped++
					continue
				}
			}

			plaintext, err := decryptOldFn(s.Ciphertext, s.Nonce)
			if err != nil {
				if decryptCurrentFn != nil {
					if _, currentErr := decryptCurrentFn(s.Ciphertext, s.Nonce); currentErr == nil {
						skipped++
						continue
					}
				}
				return fmt.Errorf("decrypt secret %s: %w", s.Name, err)
			}

			newCiphertext, newNonce, err := encryptFn(plaintext)
			if err != nil {
				return fmt.Errorf("encrypt secret %s: %w", s.Name, err)
			}
			now := time.Now().UTC()
			if err := tx.Model(s).Select("Ciphertext", "Nonce", "Version", "LastRotatedAt").Updates(&Secret{
				Ciphertext:    newCiphertext,
				Nonce:         newNonce,
				Version:       newVersion,
				LastRotatedAt: &now,
			}).Error; err != nil {
				return err
			}
			updated++
		}
		return nil
	})
	return updated, skipped, err
}
