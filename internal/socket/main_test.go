package socket_test

import (
	"os"
	"testing"

	"github.com/lattesec/ctfjx/pkg/log"
)

func TestMain(m *testing.M) {
	logger := log.NewLogger("test")
	_ = logger.SetLevel(log.TRACE)
	log.DefaultLogger.Store(logger)

	if err := logger.Start(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
