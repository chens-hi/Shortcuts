package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type Logger struct {
	writer *log.Logger
}

// New 创建日志器。设置了 SHORTCUTS_LOG_DIR 时按天写入该目录下的
// <SHORTCUTS_LOG_NAME>_YYYYMMDD.log（名字默认 server），否则仍然写 stderr。
func New() *Logger {
	var out io.Writer = os.Stderr
	if dir := strings.TrimSpace(os.Getenv("SHORTCUTS_LOG_DIR")); dir != "" {
		name := strings.TrimSpace(os.Getenv("SHORTCUTS_LOG_NAME"))
		if name == "" {
			name = "server"
		}
		out = &dailyWriter{dir: dir, name: name, now: time.Now}
	}
	return &Logger{writer: log.New(out, "", log.LstdFlags|log.Lmicroseconds)}
}

func (l *Logger) Info(message string, fields ...any) {
	l.writer.Println("INFO", message, formatFields(fields...))
}

func (l *Logger) Warn(message string, fields ...any) {
	l.writer.Println("WARN", message, formatFields(fields...))
}

func (l *Logger) Error(message string, fields ...any) {
	l.writer.Println("ERROR", message, formatFields(fields...))
}

func (l *Logger) Std() *log.Logger {
	return l.writer
}

func formatFields(fields ...any) string {
	parts := make([]string, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		parts = append(parts, fmt.Sprintf("%v=%v", fields[i], fields[i+1]))
	}
	return strings.Join(parts, " ")
}

func SanitizeOutput(output string, max int) string {
	output = strings.TrimSpace(output)
	if max <= 0 || len(output) <= max {
		return output
	}
	return output[:max] + "..."
}
