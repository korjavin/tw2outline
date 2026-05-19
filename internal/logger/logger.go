package logger

import (
	"log"
	"strings"
)

// Logger provides different log levels.
type Logger struct {
	level string
}

// New creates a new logger with the specified level.
func New(level string) *Logger {
	return &Logger{
		level: strings.ToUpper(level),
	}
}

// Debug logs debug messages.
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level == "DEBUG" {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Info logs info messages.
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level == "DEBUG" || l.level == "INFO" {
		log.Printf("[INFO] "+format, v...)
	}
}

// Warn logs warning messages.
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level == "DEBUG" || l.level == "INFO" || l.level == "WARN" {
		log.Printf("[WARN] "+format, v...)
	}
}

// Error logs error messages.
func (l *Logger) Error(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}

// Fatal logs fatal messages and exits.
func (l *Logger) Fatal(format string, v ...interface{}) {
	log.Fatalf("[FATAL] "+format, v...)
}
