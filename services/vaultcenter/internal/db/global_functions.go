package db

import "fmt"

func (d *DB) SaveGlobalFunction(fn *GlobalFunction) error {
	if fn == nil {
		return fmt.Errorf("global function is required")
	}
	if fn.Name == "" {
		return fmt.Errorf("function name is required")
	}
	if fn.FunctionHash == "" {
		return fmt.Errorf("function_hash is required")
	}
	if fn.Command == "" {
		return fmt.Errorf("command is required")
	}
	if fn.VarsJSON == "" {
		fn.VarsJSON = "{}"
	}
	return d.conn.Save(fn).Error
}

func (d *DB) GetGlobalFunction(name string) (*GlobalFunction, error) {
	return dbFirst[GlobalFunction](d, "global function "+name+" not found", "name = ?", name)
}
func (d *DB) ListGlobalFunctions() ([]GlobalFunction, error) {
	var out []GlobalFunction
	if err := d.conn.Order("name ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) DeleteGlobalFunction(name string) error {
	return dbDeleteWhere[GlobalFunction](d, "global function "+name+" not found", "name = ?", name)
}
