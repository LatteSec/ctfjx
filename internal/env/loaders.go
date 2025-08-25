package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
	"github.com/goccy/go-yaml"
	"github.com/lattesec/ctfjx/internal/helpers/mirror"
	"github.com/lattesec/log"
)

func MustFn[T any](fn func(T) error, err error) func(T) error {
	if err != nil {
		panic(err)
	}
	return fn
}

func FromYAML[T Configurable](pth string) (func(T) error, error) {
	pth = filepath.Clean(pth)
	if pth == "." {
		return nil, ErrInvalidConfigFilename
	}

	if ext := filepath.Ext(pth); ext != "" {
		if ext == ".yaml" || ext == ".yml" {
			pth = strings.TrimSuffix(pth, ext)
		} else {
			log.Warn().
				WithMeta("scope", "env").
				WithMeta("path", pth).
				Msg("invalid config extension").Send()
			return nil, ErrInvalidConfigFilename
		}
	}

	return func(cfg T) error {
		for _, ext := range [2]string{".yml", ".yaml"} {
			cfgPath := filepath.Join(pth + ext)

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

			tmp := mirror.Fresh[T]()
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

			if err := mergo.Merge(cfg, tmp, mergo.WithOverride); err != nil {
				log.Warn().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					Msgf("failed to merge config: %v", err).Send()

				log.Debug().
					WithMeta("scope", "env").
					WithMeta("path", cfgPath).
					WithMeta("data", string(data)).
					WithMeta("merge_with", cfg).
					Msgf("failed to merge config: %v", err).Send()

				return fmt.Errorf("failed to merge config from %s: %v", cfgPath, err)
			}

			log.Info().
				WithMeta("scope", "env").
				WithMeta("path", cfgPath).
				Msgf("loaded config from %s", cfgPath).Send()
		}
		return nil
	}, nil
}

func FromYAMLConfigs[T Configurable](filename string) (func(T) error, error) {
	filename = filepath.Clean(filename)
	if filename == "." {
		return nil, ErrInvalidConfigFilename
	}

	return func(cfg T) error {
		paths := resolvePaths()

		for _, dir := range paths {
			exec, err := FromYAML[T](filepath.Join(dir, filename))
			if err != nil {
				return err
			}

			if err := exec(cfg); err != nil {
				return err
			}
		}
		return nil
	}, nil
}
