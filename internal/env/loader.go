package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
	"github.com/lattesec/ctfjx/internal/helpers/mirror"
	"github.com/lattesec/log"
	"gopkg.in/yaml.v3"
)

var (
	ErrInvalidConfigFilename = errors.New("invalid config filename")
	validConfigExtensions    = []string{".yaml", ".yml"}
)

type Loader struct {
	paths []string
}

func NewLoader() *Loader {
	paths := resolvePaths()

	log.Debug().
		WithMeta("scope", "env").
		Msgf("using config paths: %s", strings.Join(paths, ", ")).Send()

	return &Loader{paths}
}

// Load merges config files into `out` (struct pointer)
//
// Usage:
//
//	l := NewLoader()
//	l.Load("config.yaml", &config)
func (l *Loader) Load(filename string, out any) error {
	if err := mirror.IsStructPointer(out); err != nil {
		return err
	}

	filename = filepath.Base(filename)
	if filename == "." {
		return ErrInvalidConfigFilename
	}

	for _, dir := range l.paths {
		for _, ext := range validConfigExtensions {
			cfgPath := filepath.Join(dir, filename+ext)
			log.Debug().
				WithMeta("scope", "env").
				WithMeta("path", cfgPath).
				Msg("attempting to load config").Send()

			data, err := os.ReadFile(cfgPath)
			if err != nil {
				if os.IsNotExist(err) {
					log.Debug().
						WithMeta("scope", "env").
						WithMeta("path", cfgPath).
						Msg("not found").Send()
					continue
				}

				log.Error().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					Msgf("failed to read config file: %v", err).Send()

				return err
			}

			tmp := mirror.NewEmpty(out) // already a &struct{}
			if err := yaml.Unmarshal(data, tmp); err != nil {
				log.Warn().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					Msgf("failed to parse: %v", err).Send()

				log.Debug().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					WithMeta("data", string(data)).
					Msgf("failed to parse: %v", err).Send()

				return fmt.Errorf("failed to parse config from %s: %v", cfgPath, err)
			}

			if err := mergo.Merge(out, tmp, mergo.WithOverride); err != nil {
				log.Warn().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					Msgf("failed to merge config: %v", err).Send()

				log.Debug().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					WithMeta("data", string(data)).
					WithMeta("merge_with", out).
					Msgf("failed to merge config: %v", err).Send()

				return fmt.Errorf("failed to merge config from %s: %v", cfgPath, err)
			}

			log.Info().
				WithMeta("scope", "env").
				WithMeta("path", cfgPath).
				Msgf("loaded config from %s", cfgPath).Send()
		}
	}

	log.Debug().WithMeta("scope", "env").Msgf("config loaded: %#v", out).Send()
	return nil
}
