package dctoolkit

import (
    "log"
    "os"
)

type LogLevel int
const (
    // pring everything
    LevelDebug LogLevel = iota
    // print only important messages
    LevelInfo
    // print only error messages
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
        log.SetOutput(os.Stdout)
        log.SetFlags(log.LstdFlags)
        log.Printf(text, args...)
    }
}
