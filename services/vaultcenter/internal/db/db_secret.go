package db

import (
	"fmt"

	"gorm.io/gorm"
)

func (d *DB) SaveSecret(secret *Secret) error {
	return d.conn.Save(secret).Error
}

func (d *DB) GetSecretByName(name string) (*Secret, error) {
	return dbFirst[Secret](d, "secret "+name+" not found", "name = ?", name)
}
func (d *DB) GetSecretByID(id string) (*Secret, error) {
	return dbFirst[Secret](d, "secret id "+id+" not found", "id = ?", id)
}
func (d *DB) GetSecretByRef(refHash string) (*Secret, error) {
	return dbFirst[Secret](d, "secret ref "+refHash+" not found", "ref = ?", refHash)
}
func (d *DB) ListSecrets() ([]Secret, error) {
	var secrets []Secret
	err := d.conn.Order("name").Find(&secrets).Error
	return secrets, err
}

func (d *DB) DeleteSecret(name string) error {
	return dbDeleteWhere[Secret](d, "secret "+name+" not found", "name = ?", name)
}
func (d *DB) CountSecrets() (int, error) {
	var count int64
	err := d.conn.Model(&Secret{}).Count(&count).Error
	return int(count), err
}

// ReencryptAllSecrets re-encrypts all secrets with a new DEK.
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
			if err := tx.Model(s).Select("Ciphertext", "Nonce", "Version").Updates(&Secret{
				Ciphertext: newCiphertext,
				Nonce:      newNonce,
				Version:    newVersion,
			}).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}
