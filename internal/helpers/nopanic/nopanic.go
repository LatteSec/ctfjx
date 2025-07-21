package nopanic

import (
	"fmt"
	"os"
	"time"
)

func run[T any](name string, rerun bool, fn func() T) (out T) {
	for {
		var panicked bool

		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "panic in %s: %v\n", name, r)
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
