package socket

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/lattesec/log"
)

func DailWithRetry(cfg *ConnConfig) (*Conn, error) {
	var lastErr error
	for i := 0; i < cfg.MaxReconnectionAttempts; i++ {
		conn, err := net.Dial("tcp", cfg.Address)
		if err != nil {
			lastErr = err
			log.Debug().
				WithMeta("conn", cfg.Name).
				WithMeta("peer", cfg.Address).
				WithMetaf("attempt", "%d/%d", i, cfg.MaxReconnectionAttempts).
				Msgf("failed to dail: %v", err).
				Send()

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

				log.Warn().
					WithMeta("conn", cfg.Name).
					WithMeta("peer", cfg.Address).
					WithMetaf("attempt", "%d/%d", i, cfg.MaxReconnectionAttempts).
					Msgf("failed to handshake with: %v", err).
					Send()

				time.Sleep(cfg.ReconnectionDelay)
				continue
			}

			return NewConnWithRaw(tlsConn, cfg), nil
		}

		return NewConnWithRaw(conn, cfg), nil
	}

	log.Error().
		WithMeta("conn", cfg.Name).
		WithMeta("peer", cfg.Address).
		WithMetaf("attempts", "%d", cfg.MaxReconnectionAttempts).
		Msgf("failed to dial: %v", lastErr).
		Send()
	return nil, fmt.Errorf("failed to dial %s after %d attempts: %w", cfg.Address, cfg.MaxReconnectionAttempts, lastErr)
}
