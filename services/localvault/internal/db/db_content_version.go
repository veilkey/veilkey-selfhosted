package db

// GetContentVersion returns the current content version counter.
// Returns 0 if the row does not exist yet.
func (d *DB) GetContentVersion() int {
	var version int
	if err := d.conn.Raw(`SELECT version FROM content_version WHERE id = 1`).Scan(&version).Error; err != nil {
		return 0
	}
	return version
}

// BumpContentVersion atomically increments the content version counter by 1.
func (d *DB) BumpContentVersion() {
	_ = d.conn.Exec(`UPDATE content_version SET version = version + 1 WHERE id = 1`).Error
}
