package db

import "fmt"

func (d *DB) CreateSSHKey(key *SSHKey) error {
	return d.conn.Create(key).Error
}

func (d *DB) ListSSHKeys() ([]SSHKey, error) {
	var out []SSHKey
	if err := d.conn.Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) GetSSHKey(ref string) (*SSHKey, error) {
	return dbFirst[SSHKey](d, "ssh key "+ref+" not found", "ref = ?", ref)
}

func (d *DB) DeleteSSHKey(ref string) error {
	return dbDeleteWhere[SSHKey](d, "ssh key "+ref+" not found", "ref = ?", ref)
}

func (d *DB) UpdateSSHKeyHosts(ref string, hostsJSON string) error {
	result := d.conn.Model(&SSHKey{}).Where("ref = ?", ref).Update("hosts_json", hostsJSON)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("ssh key %s not found", ref)
	}
	return nil
}
