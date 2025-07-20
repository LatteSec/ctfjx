package socket

import (
	"crypto/tls"
	"errors"
	"io"
	"time"

	"github.com/lattesec/ctfjx/pkg/log"
)

var ErrAddressRequired = errors.New("address is required")

type ConnConfig struct {
	Address string // The address to connect to
	Name    string // The name of the connection. This only really holds significance in logs.

	UseTLS    bool
	TLSConfig *tls.Config

	AutoReconnect           bool
	MaxReconnectionAttempts int
	ReconnectionDelay       time.Duration // The amount of time to wait between reconnection attempts

	HeartbeatInterval time.Duration // The interval at which to send pings. Set to 0 to disable.

	MessageSendTimeout time.Duration // The maximum amount of time to wait for a message to be sent
	MessageRecvTimeout time.Duration // The maximum amount of time to wait for a message to be received

	MaxHeaderSize  uint
	MaxMessageSize uint

	Handlers map[Action]HandlerFunc // The handlers to use for each action
}

func (c *ConnConfig) Validate() error {
	if c.Address == "" {
		return ErrAddressRequired
	}
	return nil
}

var DefaultConnHandlers = map[Action]HandlerFunc{
	ActionPing: func(c *Conn, header Header, r io.Reader) {
		if err := c.sendPong(); err != nil {
			log.Errorln(c.Logf("failed to send pong: %v", err))
		}
	},
	ActionPong: func(c *Conn, header Header, r io.Reader) {
		select {
		case c.pongCh <- struct{}{}:
		default:
		}
	},
}

func DefaultConnConfig(address, name string, tlsCfg *tls.Config) *ConnConfig {
	handlers := make(map[Action]HandlerFunc, len(DefaultConnHandlers))
	for k, v := range DefaultConnHandlers {
		handlers[k] = v
	}

	return &ConnConfig{
		Address: address,
		Name:    name,

		UseTLS:    tlsCfg != nil,
		TLSConfig: tlsCfg,

		AutoReconnect:           true,
		MaxReconnectionAttempts: 10,
		ReconnectionDelay:       5 * time.Second,

		HeartbeatInterval: 10 * time.Second,

		MessageSendTimeout: 5 * time.Second,
		MessageRecvTimeout: 5 * time.Second,

		MaxHeaderSize:  1 << 20, // 1MB
		MaxMessageSize: 4 << 20, // 4MB

		Handlers: handlers,
	}
}
