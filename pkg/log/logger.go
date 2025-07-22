package log

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lattesec/ctfjx/internal/helpers/cleanup"
	"github.com/lattesec/ctfjx/internal/helpers/nopanic"
)

type ILogger interface {
	GetLevel() Level
	SetLevel(level Level) error

	IsRunning() bool
	Start() error
	Close()

	Log(msg LogMessage)
	Fatal(msg LogMessage)

	GetName() string
	SetName(name string)

	GetLogfileLocation() (dir, base string)
	SetLogfileLocation(dir, base string) error

	GetMaxFileSize() int64
	SetMaxFileSize(size int64)

	GetStdout() io.Writer
	SetStdout(w io.Writer)

	GetStderr() io.Writer
	SetStderr(w io.Writer)
}

type Logger struct {
	mu     sync.RWMutex
	muFile sync.RWMutex

	level Level  // defaults to WARN
	name  string // the name of the logger

	filename    string // the filename to write logs to. leave empty to disable file writes.
	fileDir     string // the directory to write logs to. defaults to pwd.
	filePtr     atomic.Pointer[os.File]
	maxFileSize int64  // exceeding this will trigger a log rotation. defaults to 10MB. set to 0 to disable rotations.
	cleanupId   uint64 // cleanup id

	stdout io.Writer // defaults to os.Stdout.
	stderr io.Writer // defaults to os.Stderr.

	logCh     chan string   // the first character of the string will be 0 or 1. 0=stdout, 1=stderr
	logfileCh chan string   // the first character of the string will be 0 or 1. 0=stdout, 1=stderr
	closeCh   chan struct{} // closes the log writer.
}

func NewLogger(name string) *Logger {
	return &Logger{
		level:       WARN,
		name:        name,
		maxFileSize: 10 << 20, // 10MB
		closeCh:     nil,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		logCh:       make(chan string, 1<<20),
		logfileCh:   make(chan string, 1<<20),
	}
}

func (l *Logger) GetLevel() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

func (l *Logger) SetLevel(level Level) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if level < TRACE || level > QUIET {
		return ErrInvalidLogLevel
	}
	l.level = level
	return nil
}

func (l *Logger) IsRunning() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closeCh != nil
}

func (l *Logger) Start() error {
	l.mu.Lock()
	if l.closeCh != nil {
		return ErrAlreadyStarted
	}
	l.closeCh = make(chan struct{})
	l.cleanupId = cleanup.Register(func() error { l.Close(); return nil })
	name := l.name
	l.mu.Unlock()

	_, base := l.GetLogfileLocation()
	if base != "." {
		go nopanic.NoPanicReRunVoid(name+" log file writer", l.fileWriter)
		go nopanic.NoPanicReRunVoid(name+" log file rotater", l.logRotater)
	}

	go nopanic.NoPanicReRunVoid(name+" log I/O writer", l.logWriter)
	return nil
}

func (l *Logger) Close() {
	if !l.IsRunning() {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	close(l.closeCh)
	l.closeCh = nil

	l.closeLogFile()
	cleanup.Unregister(l.cleanupId)
	l.cleanupId = 0
}

func (l *Logger) Log(msg *LogMessage) {
	l.mu.RLock()
	if msg.Level < l.level {
		return
	}

	if (l.level == TRACE && msg.Level >= ERROR) || msg.Level == TRACE {
		if msg.trace == "" {
			msg.WithTraceStack()
		}
		if msg.caller == "" {
			msg.WithCaller()
		}
	}

	shouldWriteToIO := l.level < QUIET
	name := l.name
	l.mu.RUnlock()

	logLine := msg.String()
	if msg.Level >= WARN {
		logLine = "1" + logLine
	} else {
		logLine = "0" + logLine
	}

	if shouldWriteToIO {
		select {
		case l.logCh <- logLine:
		case <-l.closeCh:
		default: // drop if buffer is full
		}
	}

	if l.filePtr.Load() != nil {
		select {
		case l.logfileCh <- logLine:
		case <-l.closeCh:
		default: // drop if buffer is full
		}
	}
}

func (l *Logger) Fatal(msg *LogMessage) {
	l.Log(msg)
	cleanup.RunErrorCleanup()
	cleanup.RunCleanup()
	os.Exit(1)
}

func (l *Logger) GetName() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.name
}

func (l *Logger) SetName(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.name = name
}

func (l *Logger) GetLogfileLocation() (dir, base string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.fileDir, l.filename
}

func (l *Logger) SetLogfileLocation(dir, base string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	dir = filepath.Clean(dir)
	base = filepath.Clean(strings.TrimSuffix(filepath.Base(base), ".log"))

	if dir != "." && base == "." {
		return ErrMissingLogFilename
	}
	if base != "." {
		base += ".log"
	}

	l.fileDir = dir
	l.filename = base

	return nil
}

func (l *Logger) closeLogFile() {
	l.muFile.Lock()
	defer l.muFile.Unlock()
	close(l.logfileCh)
	ptr := l.filePtr.Load()
	if ptr != nil {
		_ = ptr.Close()
		l.filePtr.Store(nil)
	}
}

func (l *Logger) ensureLogDir() error {
	if l.fileDir == "." {
		return nil
	}

	return os.MkdirAll(filepath.Clean(l.fileDir), 0o700)
}

func (l *Logger) ensureLogFile() (*os.File, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.filename == "." {
		return nil, ErrNoLogFileConfigured
	}
	if err := l.ensureLogDir(); err != nil {
		return nil, err
	}

	logfileLocation := filepath.Join(l.fileDir, l.filename)
	if logfileLocation == "." {
		return nil, ErrNoLogFileConfigured
	}

	stat, err := os.Stat(logfileLocation)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if os.IsNotExist(err) {
		return l.openLogFile()
	}

	if stat.IsDir() {
		return nil, ErrFoundDirWhenExpectingFile
	}

	return l.openLogFile()
}

func (l *Logger) openLogFile() (*os.File, error) {
	return os.OpenFile(
		filepath.Join(l.fileDir, l.filename),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
}

func (l *Logger) logWriter() {
	for {
		select {
		case line := <-l.logCh:
			if line[0] == '0' {
				fmt.Fprint(l.stdout, line[1:])
			} else {
				fmt.Fprint(l.stderr, line[1:])
			}
		case <-l.closeCh:
			return
		}
	}
}

func (l *Logger) fileWriter() {
	l.muFile.Lock()
	if l.logfileCh == nil {
		l.logfileCh = make(chan string, 1<<20)
	}
	l.muFile.Unlock()

	logfile, err := l.ensureLogFile()
	if err != nil {
		Errorln("failed to open log file:", err)
		return
	}
	l.filePtr.Store(logfile)

	for {
		select {
		case line := <-l.logfileCh:
			l.muFile.Lock()
			_, err := l.filePtr.Load().WriteString(line[1:])
			l.muFile.Unlock()
			if err != nil {
				Errorln("failed to write to log file:", err)
				return
			}
		case <-l.closeCh:
			return
		}
	}
}

func (l *Logger) logRotater() {
	l.mu.RLock()
	if l.maxFileSize == 0 {
		l.mu.RUnlock()
		return
	}
	l.mu.RUnlock()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.RLock()
			logPath := filepath.Join(l.fileDir, l.filename)
			l.mu.RUnlock()

			info, err := os.Stat(logPath)
			if err != nil {
				if os.IsNotExist(err) {
					if _, err := l.ensureLogFile(); err != nil {
						Errorln("failed to recreate missing log file, killing rotation:", err)
						return
					}
					continue
				}
				Errorln("failed to stat log file:", err)
				return
			}

			if info.Size() <= l.maxFileSize {
				continue
			}

			l.muFile.Lock()

			rotatedName := fmt.Sprintf("%s-%s.gz", l.filename, time.Now().UTC().Format("2006-01-02_15-04-05"))
			rotatedPath := filepath.Join(l.fileDir, rotatedName)

			original, err := os.Open(filepath.Clean(logPath))
			if err != nil {
				l.muFile.Unlock()
				Errorln("failed to open log for rotation:", err)
				continue
			}

			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, err = io.Copy(gz, original)
			_ = original.Close()
			_ = gz.Close()
			if err != nil {
				l.muFile.Unlock()
				Errorln("failed to compress rotated log:", err)
				continue
			}

			if err := os.WriteFile(rotatedPath, buf.Bytes(), 0o600); err != nil {
				l.muFile.Unlock()
				Errorln("failed to write rotated log file:", err)
				continue
			}

			if err := os.Truncate(logPath, 0); err != nil {
				Errorln("failed to truncate original log after rotation:", err)
			}

			l.muFile.Unlock()

		case <-l.closeCh:
			return
		}
	}
}
