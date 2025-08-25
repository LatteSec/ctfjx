package env

import (
	"errors"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/lattesec/ctfjx/internal/helpers/mirror"
	"github.com/lattesec/ctfjx/internal/helpers/nopanic"
	"github.com/lattesec/log"
)

var (
	ErrInvalidConfigFilename = errors.New("invalid config filename")
	validConfigExtensions    = []string{".yaml", ".yml"}
)

type Configurable interface {
	Validate() error
}

// Where T is a struct pointer
type Loader[T Configurable] struct {
	cfgValue  atomic.Value
	callbacks []func(T) error
}

func NewLoader[T Configurable]() *Loader[T] {
	return &Loader[T]{}
}

func (l *Loader[T]) RegisterCallback(cb ...func(T) error) {
	l.callbacks = append(l.callbacks, cb...)
}

func (l *Loader[T]) Current() T {
	v := l.cfgValue.Load()
	if v == nil {
		var zero T
		return zero
	}
	return v.(T)
}

func (l *Loader[T]) Set(cfg T) {
	l.cfgValue.Store(cfg)
}

// AutoReload watches for SIGHUP
func (l *Loader[T]) AutoReload() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)

	go func() {
		for range ch {
			log.Info().
				WithMeta("scope", "env").
				Msg("received SIGHUP, reloading config").Send()

			err := nopanic.NoPanicRun("env-nohup-reload", func() error {
				return l.Load()
			})
			if err != nil {
				log.Error().
					WithMeta("scope", "env").
					Msgf("failed to reload config: %v", err).Send()
			}
		}
	}()
}

func (l *Loader[T]) Load() error {
	cfg := mirror.Fresh[T]().(T) // *Cfg
	log.Debug().Msgf("%#v", cfg).Send()
	for _, cb := range l.callbacks {
		if err := cb(cfg); err != nil {
			return err
		}
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	l.Set(cfg)
	log.Debug().WithMeta("scope", "env").Msgf("config loaded: %#v", cfg).Send()
	return nil
}
