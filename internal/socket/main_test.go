package socket_test

import (
	"os"
	"testing"

	"github.com/lattesec/ctfjx/pkg/log"
)

func TestMain(m *testing.M) {
	if err := log.Init("", "", log.TRACE); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
