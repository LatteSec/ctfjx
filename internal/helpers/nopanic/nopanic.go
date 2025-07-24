package nopanic

import (
	"time"

	"github.com/lattesec/log"
)

func run[T any](name string, rerun bool, fn func() T) (out T) {
	for {
		var panicked bool

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Msgf("panic in %s: %v", name, r).Send()
					panicked = true
				}
			}()

			out = fn()
		}()

		if !panicked || !rerun {
			return
		}

		time.Sleep(1 * time.Second)
	}
}

func NoPanicRun[T any](name string, fn func() T) (out T) {
	return run(name, false, fn)
}

func NoPanicRunVoid(name string, fn func()) {
	run(name, false, func() any {
		fn()
		return nil
	})
}

func NoPanicReRun[T any](name string, fn func() T) (out T) {
	return run(name, true, fn)
}

func NoPanicReRunVoid(name string, fn func()) {
	run(name, true, func() any {
		fn()
		return nil
	})
}
