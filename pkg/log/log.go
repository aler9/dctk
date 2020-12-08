package log

import (
	golog "log"
)

// Level is the log level.
type Level int

// Log levels.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
)

// Log writes a log entry.
func Log(logLevel Level, level Level, text string, args ...interface{}) {
	if level < logLevel {
		return
	}
	golog.Printf(text, args...)
}
