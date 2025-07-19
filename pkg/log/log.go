package log

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
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
	mu      sync.Mutex
	level   Level     = WARN
	logfile *os.File  = nil
	stdout  io.Writer = os.Stdout
	stderr  io.Writer = os.Stderr

	levelNames = [6]string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "QUIET"}
)

func SetLevel(l Level) error {
	switch l {
	case TRACE, DEBUG, INFO, WARN, ERROR, QUIET:
		level = l
		Debugf("set log level to %d\n", l)
		return nil
	}
	return fmt.Errorf("invalid log level: %d", l)
}

func GetLevel() Level {
	return level
}

func Init(filePath string, lvl Level) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	logfile = f
	level = lvl
	return nil
}

func log(lvl Level, msg string) {
	ts := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s [%s] %s\n", ts, levelNames[lvl], msg)

	mu.Lock()
	defer mu.Unlock()

	if logfile != nil {
		_, _ = logfile.Write([]byte(line))
	}

	if lvl < level {
		return
	}

	switch lvl {
	case TRACE, INFO:
		fmt.Fprint(stdout, line)
	case WARN, ERROR:
		fmt.Fprint(stderr, line)
	}
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
