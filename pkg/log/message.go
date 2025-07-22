package log

import (
	"fmt"
	"strings"
	"time"

	"github.com/lattesec/ctfjx/internal/helpers/debughelper"
)

type LogMessage struct {
	Timestamp time.Time         // timestamp
	Level     Level             // log level
	Msg       string            // log message
	Meta      map[string]string // log metadata

	trace  string // stack trace (optional)
	caller string // caller (optional)
}

// NewLogMessage
//
// Creates a new LogMessage
func NewLogMessage(level Level, msg string) *LogMessage {
	return &LogMessage{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Msg:       msg,
		Meta:      make(map[string]string),
	}
}

func (lm *LogMessage) WithMeta(key string, value any) *LogMessage {
	lm.Meta[key] = fmt.Sprintf("%v", value)
	return lm
}

func (lm *LogMessage) WithMetaf(key, format string, v ...any) *LogMessage {
	lm.Meta[key] = fmt.Sprintf(format, v...)
	return lm
}

func (lm *LogMessage) WithTraceStack() *LogMessage {
	lm.trace = debughelper.TraceStack()
	return lm
}

func (lm *LogMessage) WithCaller() *LogMessage {
	lm.caller = debughelper.TraceCaller()
	return lm
}

func (lm *LogMessage) String(loggerName string) string {
	var metaStr string
	if len(lm.Meta) > 0 {
		meta := make([]string, 0, len(lm.Meta))
		for k, v := range lm.Meta {
			meta = append(meta, fmt.Sprintf("%s=%s", k, v))
		}
		metaStr = fmt.Sprintf(" {%s}", strings.Join(meta, ", "))
	}

	var debugStr string
	if lm.trace != "" || lm.caller != "" {
		debugStr = fmt.Sprintf("\n==== DEBUG ====\nCaller: %s\nTrace: %s", lm.caller, lm.trace) + "===== END =====\n\n"
	}

	return strings.TrimSuffix(fmt.Sprintf("%s [%s] %s: %s%s%s",
		lm.Timestamp.Format(time.RFC3339Nano),
		levelNames[lm.Level],
		loggerName,
		strings.TrimSuffix(lm.Msg, "\n"),
		metaStr,
		debugStr,
	), "\n") + "\n"
}
