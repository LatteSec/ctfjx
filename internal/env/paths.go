package env

import (
	"os"
	"path/filepath"
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
// /etc/ctfjx/
func resolvePaths() []string {
	paths := []string{filepath.Join("/etc/", CTFJX_CONFIG_DIR_NAME)}

	if cfgDir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfgDir, CTFJX_CONFIG_DIR_NAME))
	}

	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, CTFJX_CWD_CONFIG_DIR))
	}

	if p := os.Getenv(CTFJX_CONFIG_DIR_ENV); p != "" {
		paths = append(paths, p)
	}

	return paths
}
