package log

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lattesec/ctfjx/internal/helpers/nopanic"
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

	logLevel Level = WARN

	logFilename string
	logFileDir  string
	logfilePtr  atomic.Pointer[os.File]

	stdout  io.Writer = os.Stdout
	stderr  io.Writer = os.Stderr
	maxSize int64     = 10 << 20 // 10MB

	logCh     = make(chan string, 1<<20) // The first character of the string will be 0 or 1. 0=stdout, 1=stderr
	logfileCh = make(chan string, 1<<20) // The first character of the string will be 0 or 1. 0=stdout, 1=stderr
	closeCh   chan struct{}

	mu     sync.RWMutex
	muFile sync.RWMutex

	ErrAlreadyInitialized        = errors.New("already initialized")
	ErrInvalidLogLevel           = errors.New("invalid log level")
	ErrMissingLogFilename        = errors.New("missing log filename")
	ErrNoLogFileConfigured       = errors.New("no log file configured")
	ErrFoundDirWhenExpectingFile = errors.New("found directory when expecting file")
)

func GetLevel() Level {
	mu.RLock()
	defer mu.RUnlock()
	return logLevel
}

func Init(dir, filename string, lvl Level) error {
	mu.Lock()
	if lvl < TRACE || lvl > QUIET {
		return ErrInvalidLogLevel
	}
	logLevel = lvl

	if closeCh != nil {
		return ErrAlreadyInitialized
	}
	closeCh = make(chan struct{})

	logFileDir = filepath.Clean(dir)
	logFilename = filepath.Clean(strings.TrimSuffix(filepath.Base(filename), ".log"))

	if logFileDir != "." && logFilename == "." {
		return ErrMissingLogFilename
	}
	if logFilename != "." {
		logFilename += ".log"
	}
	mu.Unlock()

	if logFilename != "." {
		go nopanic.NoPanicReRunVoid("log file writer", fileWriter)
		go nopanic.NoPanicReRunVoid("log file rotater", logRotater)
	}

	go nopanic.NoPanicReRunVoid("log I/O writer", logWriter)

	return nil
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	close(closeCh)
	closeLogFile()
}

func closeLogFile() {
	muFile.Lock()
	defer muFile.Unlock()
	close(logfileCh)
	ptr := logfilePtr.Load()
	if ptr != nil {
		_ = ptr.Close()
		logfilePtr.Store(nil)
	}
}

func logWriter() {
	for {
		select {
		case line := <-logCh:
			if line[0] == '0' {
				fmt.Fprint(stdout, line[1:])
			} else {
				fmt.Fprint(stderr, line[1:])
			}
		case <-closeCh:
			return
		}
	}
}

func fileWriter() {
	muFile.Lock()
	if logfileCh == nil {
		logfileCh = make(chan string, 1<<20)
	}
	muFile.Unlock()

	logfile, err := ensureLogFile()
	if err != nil {
		Errorln("failed to open log file:", err)
		return
	}
	logfilePtr.Store(logfile)

	for {
		select {
		case line := <-logfileCh:
			muFile.Lock()
			_, err := logfilePtr.Load().WriteString(line[1:])
			muFile.Unlock()
			if err != nil {
				Errorln("failed to write to log file:", err)
				return
			}
		case <-closeCh:
			return
		}
	}
}

func logRotater() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mu.RLock()
			logPath := filepath.Join(logFileDir, logFilename)
			mu.RUnlock()

			info, err := os.Stat(logPath)
			if err != nil {
				if os.IsNotExist(err) {
					if _, err := ensureLogFile(); err != nil {
						Errorln("failed to recreate missing log file, killing rotation:", err)
						return
					}
					continue
				}
				Errorln("failed to stat log file:", err)
				return
			}

			if info.Size() <= maxSize {
				continue
			}

			muFile.Lock()

			rotatedName := fmt.Sprintf("%s-%s.gz", logFilename, time.Now().UTC().Format("2006-01-02_15-04-05"))
			rotatedPath := filepath.Join(logFileDir, rotatedName)

			original, err := os.Open(filepath.Clean(logPath))
			if err != nil {
				muFile.Unlock()
				Errorln("failed to open log for rotation:", err)
				continue
			}

			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, err = io.Copy(gz, original)
			_ = original.Close()
			_ = gz.Close()
			if err != nil {
				muFile.Unlock()
				Errorln("failed to compress rotated log:", err)
				continue
			}

			if err := os.WriteFile(rotatedPath, buf.Bytes(), 0o600); err != nil {
				muFile.Unlock()
				Errorln("failed to write rotated log file:", err)
				continue
			}

			if err := os.Truncate(logPath, 0); err != nil {
				Errorln("failed to truncate original log after rotation:", err)
			}

			muFile.Unlock()

		case <-closeCh:
			return
		}
	}
}

func ensureLogDir() error {
	if logFileDir == "." {
		return nil
	}

	return os.MkdirAll(filepath.Clean(logFileDir), 0o700)
}

func ensureLogFile() (*os.File, error) {
	mu.RLock()
	defer mu.RUnlock()

	if logFilename == "." {
		return nil, ErrNoLogFileConfigured
	}
	if err := ensureLogDir(); err != nil {
		return nil, err
	}

	logfileLocation := filepath.Join(logFileDir, logFilename)
	if logfileLocation == "." {
		return nil, ErrNoLogFileConfigured
	}

	stat, err := os.Stat(logfileLocation)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if os.IsNotExist(err) {
		return openLogFile(logfileLocation)
	}

	if stat.IsDir() {
		return nil, ErrFoundDirWhenExpectingFile
	}

	return openLogFile(logfileLocation)
}

func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}

func traceCaller() string {
	pc, file, line, ok := runtime.Caller(3)
	if !ok {
		return "???"
	}
	short := filepath.Base(file)
	fn := runtime.FuncForPC(pc).Name()
	return fmt.Sprintf("trace: %s:%d (%s)", short, line, fn)
}

func traceStack() string {
	buf := make([]byte, 4<<10)
	n := runtime.Stack(buf, false)
	return "stack:\n" + string(buf[:n])
}

func log(lvl Level, msg string) {
	mu.RLock()
	if lvl < logLevel {
		return
	}

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	lines := []string{}

	if logLevel == TRACE && (lvl == TRACE || lvl == ERROR) {
		lines = append(lines,
			fmt.Sprintf("%s [TRACE] %s", ts, traceCaller()),
			fmt.Sprintf("%s [TRACE] %s", ts, traceStack()),
		)
	}

	lines = append(lines, fmt.Sprintf("%s [%s] %s", ts, levelNames[lvl], msg))
	shouldWriteToIO := logLevel < QUIET
	mu.RUnlock()

	full := strings.Join(lines, "\n")
	if lvl >= WARN {
		full = "1" + full
	} else {
		full = "0" + full
	}

	if shouldWriteToIO {
		select {
		case logCh <- full:
		case <-closeCh:
		default: // drop logs when buffer is full
		}
	}

	if logfilePtr.Load() != nil {
		select {
		case logfileCh <- full:
		case <-closeCh:
		default: // drop logs when buffer is full
		}
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
