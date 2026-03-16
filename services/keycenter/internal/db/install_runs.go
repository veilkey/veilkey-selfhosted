package db

import "fmt"

func (d *DB) SaveInstallRun(run *InstallRun) error {
	if run == nil {
		return fmt.Errorf("install run is required")
	}
	if run.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	if run.RunKind == "" {
		run.RunKind = "apply"
	}
	if run.Status == "" {
		run.Status = "pending"
	}
	if run.CommandJSON == "" {
		run.CommandJSON = "[]"
	}
	if run.ValidationJSON == "" {
		run.ValidationJSON = "{}"
	}
	return d.conn.Save(run).Error
}

func (d *DB) GetInstallRun(runID string) (*InstallRun, error) {
	var run InstallRun
	if err := d.conn.First(&run, "run_id = ?", runID).Error; err != nil {
		return nil, fmt.Errorf("install run %s not found", runID)
	}
	return &run, nil
}

func (d *DB) GetLatestInstallRun() (*InstallRun, error) {
	var run InstallRun
	if err := d.conn.Order("created_at DESC").First(&run).Error; err != nil {
		return nil, fmt.Errorf("no install run found")
	}
	return &run, nil
}

func (d *DB) ListInstallRuns(limit int) ([]InstallRun, error) {
	if limit <= 0 {
		limit = 20
	}
	var runs []InstallRun
	if err := d.conn.Order("created_at DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}
