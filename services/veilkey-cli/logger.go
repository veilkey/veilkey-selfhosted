package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type LogEntry struct {
	Timestamp  string `json:"timestamp"`
	VeilKey    string `json:"veilkey"`
	Pattern    string `json:"pattern"`
	Confidence int    `json:"confidence"`
	Preview    string `json:"preview"`
}

type SessionLogger struct {
	path string
}

func NewSessionLogger(path string) *SessionLogger {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	return &SessionLogger{path: path}
}

func (l *SessionLogger) Log(veilkey, pattern string, confidence int, preview string) {
	entry := LogEntry{
		Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
		VeilKey:    veilkey,
		Pattern:    pattern,
		Confidence: confidence,
		Preview:    preview,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	f.Write(data)
	f.Write([]byte("\n"))
}

func (l *SessionLogger) ReadEntries() ([]LogEntry, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}

	var entries []LogEntry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (l *SessionLogger) Count() int {
	entries, err := l.ReadEntries()
	if err != nil {
		return 0
	}
	return len(entries)
}

func (l *SessionLogger) Clear() error {
	return os.Remove(l.path)
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
