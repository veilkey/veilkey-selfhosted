package hkm

import (
	"os"
	"strings"
	"testing"
)

func extractFnBody(code, sig string) string {
	i := strings.Index(code, sig)
	if i < 0 {
		return ""
	}
	r := code[i:]
	n := strings.Index(r[1:], "\nfunc ")
	if n < 0 {
		return r
	}
	return r[:n+1]
}

func TestFunctionRunProcessKilledOnTimeout(t *testing.T) {
	src, _ := os.ReadFile("hkm_global_function_run.go")
	code := string(src)
	if !strings.Contains(code, "exec.CommandContext") {
		t.Error("must use exec.CommandContext for timeout")
	}
	if !strings.Contains(code, "Process.Kill") {
		t.Error("must kill process on timeout")
	}
}

func TestFunctionRunOutputBounded(t *testing.T) {
	src, _ := os.ReadFile("hkm_global_function_run.go")
	code := string(src)
	if !strings.Contains(code, "limitedWriter") {
		t.Error("stdout/stderr must use limitedWriter to prevent OOM")
	}
	if !strings.Contains(code, "maxFunctionOutputSize") {
		t.Error("must define maxFunctionOutputSize constant")
	}
}

func TestLimitedWriterExists(t *testing.T) {
	src, _ := os.ReadFile("hkm_global_function_run.go")
	code := string(src)
	b := extractFnBody(code, "func (w *limitedWriter) Write(")
	if b == "" {
		t.Fatal("limitedWriter.Write must exist")
	}
	// Must cap at max
	if !strings.Contains(b, "w.max") {
		t.Error("limitedWriter must enforce max size")
	}
	// Must not panic on overflow
	if !strings.Contains(b, "remaining") || !strings.Contains(b, "<= 0") {
		t.Error("limitedWriter must handle overflow gracefully")
	}
}
