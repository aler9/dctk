package dctoolkit

import (
	"log"
	"os"
)

// LogLevel contains the available log levels.
type LogLevel int

const (
	// LevelDebug prints everything
	LevelDebug LogLevel = iota
	// LevelInfo prints only important messages
	LevelInfo
	// LevelError prints only error messages
	LevelError
)

var logLevel = LevelInfo

// SetLogLevel sets the verbosity of the library. See LogLevel for the description
// of the available levels.
func SetLogLevel(level LogLevel) {
	logLevel = level
}

func dolog(level LogLevel, text string, args ...interface{}) {
	if level >= logLevel {
		log.Printf(text, args...)
	}
}
