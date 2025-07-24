package socket_test

import (
	"os"
	"testing"

	"github.com/lattesec/log"
)

func TestMain(m *testing.M) {
	logger, _ := log.NewLogger().Name("test").WithLevel(log.DEBUG).Build()
	log.Register(logger)

	if err := logger.Start(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
