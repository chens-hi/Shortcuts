package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyWriterRollsOverAtMidnight(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 10, 21, 23, 59, 0, 0, time.UTC)
	writer := &dailyWriter{dir: dir, name: "server", now: func() time.Time { return clock }}

	if _, err := writer.Write([]byte("day1\n")); err != nil {
		t.Fatalf("write day1: %v", err)
	}
	first := filepath.Join(dir, "server_20261021.log")
	if data, err := os.ReadFile(first); err != nil || string(data) != "day1\n" {
		t.Fatalf("first day file = %q, err %v", data, err)
	}

	// 跨到第二天：必须换文件，而不是继续写旧文件。
	clock = clock.Add(2 * time.Minute)
	if _, err := writer.Write([]byte("day2\n")); err != nil {
		t.Fatalf("write day2: %v", err)
	}
	second := filepath.Join(dir, "server_20261022.log")
	if data, err := os.ReadFile(second); err != nil || string(data) != "day2\n" {
		t.Fatalf("second day file = %q, err %v", data, err)
	}
	if data, _ := os.ReadFile(first); string(data) != "day1\n" {
		t.Fatalf("first day file grew after rollover: %q", data)
	}
}

func TestDailyWriterAppendsWithinSameDay(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	writer := &dailyWriter{dir: dir, name: "agent", now: func() time.Time { return clock }}

	if _, err := writer.Write([]byte("a\n")); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if _, err := writer.Write([]byte("b\n")); err != nil {
		t.Fatalf("write b: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agent_20261021.log"))
	if err != nil || string(data) != "a\nb\n" {
		t.Fatalf("same day append = %q, err %v", data, err)
	}
}

func TestDailyWriterFallsBackWhenDirUnusable(t *testing.T) {
	// 用一个普通文件挡在目录位置上，MkdirAll 必然失败。
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	writer := &dailyWriter{dir: filepath.Join(blocker, "log"), name: "server", now: time.Now}

	n, err := writer.Write([]byte("fallback\n"))
	if err != nil || n != len("fallback\n") {
		t.Fatalf("fallback write = %d, %v", n, err)
	}
}

func TestNewWritesDailyFileWhenLogDirSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHORTCUTS_LOG_DIR", dir)
	t.Setenv("SHORTCUTS_LOG_NAME", "server")

	New().Info("hello", "k", "v")

	matches, err := filepath.Glob(filepath.Join(dir, "server_*.log"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one daily file, got %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "k=v") {
		t.Fatalf("unexpected log content: %q", string(data))
	}
}
