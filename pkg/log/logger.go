package log

import (
	"sync"
	"sync/atomic"
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

	GetFilename() string
	SetFilename(filename string)

	GetFileDir() string
	SetFileDir(dir string)

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
	maxFileSize int64 // exceeding this will trigger a log rotation. defaults to 10MB. set to 0 to disable rotations.

	stdout io.Writer // defaults to os.Stdout.
	stderr io.Writer // defaults to os.Stderr.

	logCh     chan LogMessage // the first character of the string will be 0 or 1. 0=stdout, 1=stderr
	logfileCh chan LogMessage // the first character of the string will be 0 or 1. 0=stdout, 1=stderr
	closeCh   chan struct{}   // closes the log writer.
}

