package debughelper

import (
	"fmt"
	"path/filepath"
	"runtime"
)

func TraceCaller() string {
	pc, file, line, ok := runtime.Caller(3)
	if !ok {
		return "???"
	}
	short := filepath.Base(file)
	fn := runtime.FuncForPC(pc).Name()
	return fmt.Sprintf("trace: %s:%d (%s)", short, line, fn)
}

func TraceStack() string {
	buf := make([]byte, 4<<10)
	n := runtime.Stack(buf, false)
	return "stack:\n" + string(buf[:n])
}
