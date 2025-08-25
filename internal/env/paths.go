package env

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lattesec/log"
)

const (
	CTFJX_CONFIG_DIR_NAME = "ctfjx"

	CTFJX_CONFIG_DIR_ENV = "CTFJX_CONFIG_DIR"
	CTFJX_CWD_CONFIG_DIR = ".ctfjx"
)

// In decreasing priority order
//
// Check in these locations:
// $CTFJX_CONFIG_DIR/
// ./.ctfjx/
// $XDG_CONFIG_HOME/ctfjx/ OR $HOME/.config/ctfjx/
// /etc/ctfjx/ OR %APPDATA%/ctfjx/
func resolvePaths() []string {
	var paths []string

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			paths = append(paths, filepath.Join(appData, CTFJX_CONFIG_DIR_NAME))
		}
	} else {
		paths = append(paths, "/etc/"+CTFJX_CONFIG_DIR_NAME)
	}

	if cfgDir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfgDir, CTFJX_CONFIG_DIR_NAME))
	}

	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, CTFJX_CWD_CONFIG_DIR))
	}

	if p := os.Getenv(CTFJX_CONFIG_DIR_ENV); p != "" {
		paths = append(paths, p)
	}

	log.Debug().
		WithMeta("scope", "env").
		Msgf("using config paths: %s", strings.Join(paths, ", ")).Send()

	return paths
}
