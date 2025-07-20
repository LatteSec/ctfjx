package socket

import (
	"crypto/tls"
	"errors"
	"net"
)

var ErrTLSMissingConfig = errors.New("tls config is required")

// Wraps a net.Conn in a TLS connection
func WrapTLS(conn net.Conn, cfg *tls.Config) (net.Conn, error) {
	if cfg == nil {
		return nil, ErrTLSMissingConfig
	}

	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.Handshake(); err != nil {
		return nil, err
	}

	return tlsConn, nil
}
