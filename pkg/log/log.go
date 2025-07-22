package log

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
)

// Log Level
type Level int

// Log Levels
//
// Arranged from most to least verbose
const (
	TRACE Level = iota
	DEBUG
	INFO
	WARN
	ERROR
	QUIET
)

var (
	levelNames = [6]string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "QUIET"}

	DefaultLogger atomic.Pointer[Logger]

	ErrAlreadyStarted            = errors.New("already started")
	ErrInvalidLogLevel           = errors.New("invalid log level")
	ErrMissingLogFilename        = errors.New("missing log filename")
	ErrNoLogFileConfigured       = errors.New("no log file configured")
	ErrFoundDirWhenExpectingFile = errors.New("found directory when expecting file")
)

func Log(msg *LogMessage) {
	logger := DefaultLogger.Load()
	if logger == nil {
		newLogger := NewLogger("default")
		if err := newLogger.Start(); err != nil {
			panic(fmt.Errorf("could not start default logger: %v", err))
		}

		DefaultLogger.Store(newLogger)
		logger = newLogger
		logger.Log(NewLogMessage(INFO, "default logger started"))
	}

	logger.Log(msg)
}

func log(level Level, msg string) {
	Log(NewLogMessage(level, msg))
}

func Debug(v ...any) { log(DEBUG, fmt.Sprint(v...)) }
func Info(v ...any)  { log(INFO, fmt.Sprint(v...)) }
func Warn(v ...any)  { log(WARN, fmt.Sprint(v...)) }
func Error(v ...any) { log(ERROR, fmt.Sprint(v...)) }
func Fatal(v ...any) { log(ERROR, fmt.Sprint(v...)); os.Exit(1) }

func Debugf(format string, v ...any) { log(DEBUG, fmt.Sprintf(format, v...)) }
func Infof(format string, v ...any)  { log(INFO, fmt.Sprintf(format, v...)) }
func Warnf(format string, v ...any)  { log(WARN, fmt.Sprintf(format, v...)) }
func Errorf(format string, v ...any) { log(ERROR, fmt.Sprintf(format, v...)) }
func Fatalf(format string, v ...any) {
	log(ERROR, fmt.Sprintf(format, v...))
	os.Exit(1)
}

func Debugln(v ...any) { log(DEBUG, fmt.Sprintln(v...)) }
func Infoln(v ...any)  { log(INFO, fmt.Sprintln(v...)) }
func Warnln(v ...any)  { log(WARN, fmt.Sprintln(v...)) }
func Errorln(v ...any) { log(ERROR, fmt.Sprintln(v...)) }
func Fatalln(v ...any) {
	log(ERROR, fmt.Sprintln(v...))
	os.Exit(1)
}
