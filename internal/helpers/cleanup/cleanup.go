package cleanup

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/lattesec/ctfjx/internal/helpers/nopanic"
	"github.com/lattesec/ctfjx/pkg/log"
)

type CleanupFunc func() error

var (
	once sync.Once

	errMu      sync.Mutex
	errorIdGen uint64
	errorFns   = make(map[uint64]CleanupFunc)

	mu           sync.Mutex
	cleanupIdGen uint64
	cleanupFns   = make(map[uint64]CleanupFunc)
)

// Register registers a cleanup function
// that is called on exit
func Register(fn CleanupFunc) uint64 {
	id := atomic.AddUint64(&cleanupIdGen, 1)
	mu.Lock()
	cleanupFns[id] = fn
	mu.Unlock()
	return id
}

func Unregister(id uint64) {
	mu.Lock()
	delete(cleanupFns, id)
	mu.Unlock()
}

// RegisterError registers an error cleanup function
// that is called on error exit
func RegisterError(fn CleanupFunc) uint64 {
	id := atomic.AddUint64(&errorIdGen, 1)
	errMu.Lock()
	errorFns[id] = fn
	errMu.Unlock()
	return id
}

func UnregisterError(id uint64) {
	errMu.Lock()
	delete(errorFns, id)
	errMu.Unlock()
}

func RunErrorCleanup() {
	errMu.Lock()
	fns := make([]CleanupFunc, 0, len(errorFns))
	for _, fn := range errorFns {
		fns = append(fns, fn)
	}
	errorFns = make(map[uint64]CleanupFunc)
	atomic.StoreUint64(&errorIdGen, 0)
	errMu.Unlock()
	for i, fn := range fns {
		name := fmt.Sprintf("error cleanup %d", i)
		if err := nopanic.NoPanicRun(name, fn); err != nil {
			log.Errorf("%s failed: %v", name, err)
		}
	}
}

func RunCleanup() {
	mu.Lock()
	fns := make([]CleanupFunc, 0, len(cleanupFns))
	for _, fn := range cleanupFns {
		fns = append(fns, fn)
	}
	cleanupFns = make(map[uint64]CleanupFunc)
	atomic.StoreUint64(&cleanupIdGen, 0)
	mu.Unlock()
	for i, fn := range fns {
		name := fmt.Sprintf("cleanup %d", i)
		if err := nopanic.NoPanicRun(name, fn); err != nil {
			log.Errorf("%s failed: %v", name, err)
		}
	}
}

func Listen() {
	once.Do(func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		RunErrorCleanup()
		RunCleanup()
	})
}
