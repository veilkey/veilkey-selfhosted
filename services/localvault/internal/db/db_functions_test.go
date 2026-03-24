package db

import (
	"testing"
	"time"
)

func TestSaveFunction_Basic(t *testing.T) {
	d := newTestDB(t)
	fn := &Function{
		Name:         "test-fn",
		Scope:        "LOCAL",
		VaultHash:    "vault1",
		FunctionHash: "hash1",
		Category:     "test",
		Command:      "echo hello",
		VarsJSON:     "{}",
	}
	if err := d.SaveFunction(fn); err != nil {
		t.Fatalf("SaveFunction: %v", err)
	}

	got, err := d.GetFunction("test-fn")
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	if got.Command != "echo hello" {
		t.Errorf("Command = %q, want %q", got.Command, "echo hello")
	}
	if got.Scope != "LOCAL" {
		t.Errorf("Scope = %q, want LOCAL", got.Scope)
	}
}

func TestSaveFunction_Upsert(t *testing.T) {
	d := newTestDB(t)
	fn := &Function{
		Name: "upsert-fn", Scope: "LOCAL", VaultHash: "v1",
		FunctionHash: "h1", Command: "echo v1",
	}
	if err := d.SaveFunction(fn); err != nil {
		t.Fatalf("SaveFunction: %v", err)
	}

	fn.Command = "echo v2"
	if err := d.SaveFunction(fn); err != nil {
		t.Fatalf("SaveFunction upsert: %v", err)
	}

	got, _ := d.GetFunction("upsert-fn")
	if got.Command != "echo v2" {
		t.Errorf("Command after upsert = %q, want %q", got.Command, "echo v2")
	}
}

func TestSaveFunction_Validation(t *testing.T) {
	d := newTestDB(t)
	cases := []struct {
		name string
		fn   Function
	}{
		{"empty name", Function{Scope: "LOCAL", VaultHash: "v", FunctionHash: "h", Command: "echo"}},
		{"empty scope", Function{Name: "n", VaultHash: "v", FunctionHash: "h", Command: "echo"}},
		{"invalid scope", Function{Name: "n", Scope: "INVALID", VaultHash: "v", FunctionHash: "h", Command: "echo"}},
		{"empty vault_hash", Function{Name: "n", Scope: "LOCAL", FunctionHash: "h", Command: "echo"}},
		{"empty function_hash", Function{Name: "n", Scope: "LOCAL", VaultHash: "v", Command: "echo"}},
		{"empty command", Function{Name: "n", Scope: "LOCAL", VaultHash: "v", FunctionHash: "h"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := d.SaveFunction(&tc.fn); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestSaveFunction_ScopeNormalization(t *testing.T) {
	d := newTestDB(t)
	fn := &Function{
		Name: "scope-fn", Scope: "local", VaultHash: "v",
		FunctionHash: "h", Command: "echo",
	}
	if err := d.SaveFunction(fn); err != nil {
		t.Fatalf("SaveFunction: %v", err)
	}
	got, _ := d.GetFunction("scope-fn")
	if got.Scope != "LOCAL" {
		t.Errorf("Scope = %q, want LOCAL", got.Scope)
	}
}

func TestListFunctions(t *testing.T) {
	d := newTestDB(t)
	for i, scope := range []string{"LOCAL", "GLOBAL", "TEST"} {
		_ = d.SaveFunction(&Function{
			Name: scope + "-fn", Scope: scope, VaultHash: "v",
			FunctionHash: "h" + string(rune('0'+i)), Command: "echo",
		})
	}

	all, err := d.ListFunctions()
	if err != nil {
		t.Fatalf("ListFunctions: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListFunctions count = %d, want 3", len(all))
	}

	globals, err := d.ListFunctionsByScope("GLOBAL")
	if err != nil {
		t.Fatalf("ListFunctionsByScope: %v", err)
	}
	if len(globals) != 1 {
		t.Errorf("GLOBAL count = %d, want 1", len(globals))
	}
}

func TestDeleteFunction(t *testing.T) {
	d := newTestDB(t)
	_ = d.SaveFunction(&Function{
		Name: "del-fn", Scope: "LOCAL", VaultHash: "v",
		FunctionHash: "h", Command: "echo",
	})

	if err := d.DeleteFunction("del-fn"); err != nil {
		t.Fatalf("DeleteFunction: %v", err)
	}
	if _, err := d.GetFunction("del-fn"); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestDeleteFunction_NotFound(t *testing.T) {
	d := newTestDB(t)
	if err := d.DeleteFunction("nonexistent"); err == nil {
		t.Error("expected error for nonexistent function")
	}
}

func TestCleanupExpiredTestFunctions(t *testing.T) {
	d := newTestDB(t)

	// 만료된 TEST 함수 (2시간 전)
	_ = d.SaveFunction(&Function{
		Name: "old-test", Scope: "TEST", VaultHash: "v",
		FunctionHash: "h1", Command: "echo",
	})
	// created_at를 직접 조작
	d.conn.Model(&Function{}).Where("name = ?", "old-test").
		Update("created_at", time.Now().Add(-2*time.Hour))

	// 아직 유효한 TEST 함수
	_ = d.SaveFunction(&Function{
		Name: "new-test", Scope: "TEST", VaultHash: "v",
		FunctionHash: "h2", Command: "echo",
	})

	// LOCAL 함수 (cleanup 대상 아님)
	_ = d.SaveFunction(&Function{
		Name: "local-fn", Scope: "LOCAL", VaultHash: "v",
		FunctionHash: "h3", Command: "echo",
	})

	count, err := d.CleanupExpiredTestFunctions(time.Now())
	if err != nil {
		t.Fatalf("CleanupExpiredTestFunctions: %v", err)
	}
	if count != 1 {
		t.Errorf("cleaned up %d, want 1", count)
	}

	// old-test 삭제됨
	if _, err := d.GetFunction("old-test"); err == nil {
		t.Error("old-test should be deleted")
	}
	// new-test 남아있음
	if _, err := d.GetFunction("new-test"); err != nil {
		t.Error("new-test should still exist")
	}
	// local-fn 남아있음
	if _, err := d.GetFunction("local-fn"); err != nil {
		t.Error("local-fn should still exist")
	}
}

func TestCountFunctions(t *testing.T) {
	d := newTestDB(t)
	count, _ := d.CountFunctions()
	if count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	_ = d.SaveFunction(&Function{
		Name: "fn1", Scope: "LOCAL", VaultHash: "v",
		FunctionHash: "h1", Command: "echo",
	})
	count, _ = d.CountFunctions()
	if count != 1 {
		t.Errorf("count after save = %d, want 1", count)
	}
}

func TestFunctionLogs(t *testing.T) {
	d := newTestDB(t)
	_ = d.SaveFunction(&Function{
		Name: "log-fn", Scope: "LOCAL", VaultHash: "v",
		FunctionHash: "hlog", Command: "echo",
	})
	_ = d.DeleteFunction("log-fn")

	logs, err := d.ListFunctionLogs()
	if err != nil {
		t.Fatalf("ListFunctionLogs: %v", err)
	}
	if len(logs) < 2 {
		t.Errorf("log count = %d, want >= 2 (save + delete)", len(logs))
	}
}
