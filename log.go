package dctk

import (
    "log"
    "os"
)

type LogLevel int
const (
    LevelDebug LogLevel = iota
    LevelInfo
    LevelError
)

var logLevel = LevelInfo

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
