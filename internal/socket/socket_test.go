package socket

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConn_Reconnect(t *testing.T) {
	addr, stop := startMockServer(t, false, func(c net.Conn) {
		defer c.Close()
		time.Sleep(5 * time.Second)
	})
	defer stop()

	cfg := DefaultConnConfig(addr, "reconnect-client", nil)
	cfg.ReconnectionDelay = 10 * time.Millisecond
	cfg.HeartbeatInterval = 0

	c := NewConn(cfg)
	err := c.reconnect()
	assert.NoError(t, err, "failed to reconnect as initial connect")

	// let a few pings happen
	time.Sleep(200 * time.Millisecond)

	err = c.Close()
	assert.NoError(t, err, "failed to close connection")
}

func TestConn_PingPong(t *testing.T) {
	addr, stop := startMockServer(t, false, func(c net.Conn) {
		defer c.Close()

		cfg := DefaultConnConfig(c.RemoteAddr().String(), "ping-server", nil)
		cfg.ReconnectionDelay = 10 * time.Millisecond
		cfg.HeartbeatInterval = time.Second

		server := NewConnWithRaw(c, cfg)
		server.Listen()
	})
	defer stop()

	clientCfg := DefaultConnConfig(addr, "ping-client", nil)
	clientCfg.ReconnectionDelay = 10 * time.Millisecond
	clientCfg.HeartbeatInterval = 0
	client := NewConn(clientCfg)

	time.Sleep(2 * time.Second)

	err := client.Connect()
	assert.NoError(t, err)

	assert.NoError(t, client.sendPing(), "failed to send ping")

	select {
	case <-client.pongCh:
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive pong in time")
	}

	t.Log("closing connection from test")
	err = client.Close()
	assert.NoError(t, err)
}

// Intentionally connect to a non-TLS server with TLS enabled to force error
func TestConn_TLSWrap_Fail(t *testing.T) {
	addr, stop := startMockServer(t, false, func(c net.Conn) {
		defer c.Close()
		_, _ = c.Write([]byte("not tls"))
	})
	defer stop()

	raw, err := net.Dial("tcp", addr)
	assert.NoError(t, err, "dial failed")

	_, err = WrapTLS(raw, &tls.Config{InsecureSkipVerify: true})
	assert.Error(t, err, "expected TLS handshake to fail")

	raw.Close()
}
