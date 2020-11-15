package log

import (
	golog "log"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
)

func Log(logLevel Level, level Level, text string, args ...interface{}) {
	if level >= logLevel {
		golog.Printf(text, args...)
	}
}
