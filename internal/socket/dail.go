package socket

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/lattesec/ctfjx/pkg/log"
)

func DailWithRetry(cfg *ConnConfig) (*Conn, error) {
	var lastErr error
	for i := 0; i < cfg.MaxReconnectionAttempts; i++ {
		conn, err := net.Dial("tcp", cfg.Address)
		if err != nil {
			lastErr = err
			log.Warnf("failed to dial %s: %v\n", cfg.Address, err)
			time.Sleep(cfg.ReconnectionDelay)
			continue
		}

		if cfg.UseTLS {
			tlsConn, err := WrapTLS(conn, cfg.TLSConfig)
			if err != nil {
				lastErr = err
				if err := conn.Close(); err != nil {
					lastErr = errors.Join(lastErr, err)
				}
				log.Warnf("failed to handshake with %s: %v\n", cfg.Address, err)
				time.Sleep(cfg.ReconnectionDelay)
				continue
			}

			return NewConnWithRaw(tlsConn, cfg), nil
		}

		return NewConnWithRaw(conn, cfg), nil
	}

	return nil, fmt.Errorf("failed to dial %s after %d attempts: %w", cfg.Address, cfg.MaxReconnectionAttempts, lastErr)
}
