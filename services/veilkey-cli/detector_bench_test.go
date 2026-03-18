package main

import (
	"strings"
	"testing"
)

func BenchmarkDetectSecrets(b *testing.B) {
	d := testDetector()
	line := "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.DetectSecrets(line)
	}
}

func BenchmarkProcessLine(b *testing.B) {
	d := testDetector()
	line := "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a glpat-AbCdEfGhIjKlMnOpQrSt1234 AKIAIOSFODNNENEXAMPL"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.ProcessLine(line)
	}
}

func BenchmarkShannonEntropy(b *testing.B) {
	s := strings.Repeat("aB3$xY9!mK2@pL5#", 4) // 64 chars

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shannonEntropy(s)
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadConfig("")
		if err != nil {
			b.Fatal(err)
		}
	}
}
