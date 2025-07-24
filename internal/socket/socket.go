package socket

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/lattesec/log"
)

type ConnState uint8

const (
	ConnStateIdle ConnState = iota
	ConnStateUnknown
	ConnStateOpen
	ConnStateClosed
	ConnStateReconnecting
)

func (s ConnState) String() string {
	switch s {
	case ConnStateIdle:
		return "idle"
	case ConnStateOpen:
		return "open"
	case ConnStateClosed:
		return "closed"
	case ConnStateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

var (
	ErrInvalidHeader   = errors.New("invalid packet header")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrInvalidAction   = errors.New("invalid action")

	ErrConnectionClosed              = errors.New("connection closed")
	ErrConnectionNotEstablished      = errors.New("connection not established")
	ErrConnectionAlreadyReconnecting = errors.New("connection already reconnecting")
	ErrConnectionTLSUpgradeFailed    = errors.New("tls upgrade failed")
	ErrExhaustedReconnectAttempts    = errors.New("exhausted reconnect attempts")
)

// The packet header
type Header struct {
	Action Action
	Len    uint64 // Payload size
}

func (h *Header) MarshalBytes() ([]byte, error) {
	buf := make([]byte, 9)
	buf[0] = byte(h.Action)
	binary.BigEndian.PutUint64(buf[1:], h.Len)
	return buf, nil
}

func (h *Header) UnmarshalBytes(buf []byte) error {
	if len(buf) < 9 {
		return ErrInvalidHeader
	}

	h.Action = Action(buf[0])
	h.Len = binary.BigEndian.Uint64(buf[1:])
	return nil
}

func UnmarshalHeader(buf []byte) (Header, error) {
	var h Header
	err := h.UnmarshalBytes(buf)
	return h, err
}

type HandlerFunc func(c *Conn, header Header, r io.Reader)

type Conn struct {
	Config *ConnConfig
	logger *log.Logger

	raw      net.Conn
	state    ConnState
	lastPing time.Time

	muConn sync.RWMutex
	muSend sync.Mutex

	ReadDone chan struct{} // closes when reading is done
	pongCh   chan struct{}
}

func NewConn(cfg *ConnConfig) *Conn {
	return NewConnWithRaw(nil, cfg)
}

func NewConnWithRaw(raw net.Conn, cfg *ConnConfig) *Conn {
	return &Conn{
		Config: cfg,

		raw:      raw,
		state:    ConnStateIdle,
		lastPing: time.Now().UTC(),
	}
}

// Otherwise uses the default logger
func (c *Conn) RegisterLogger(l *log.Logger) {
	c.muConn.Lock()
	defer c.muConn.Unlock()
	c.logger = l
}

func (c *Conn) String() string {
	c.muConn.RLock()
	defer c.muConn.RUnlock()
	return c.unsafeString()
}

func (c *Conn) unsafeString() string {
	return fmt.Sprintf("{con: %s -> %s, state: %s, tls: %v, lastPing: %s}",
		c.Config.Name, c.Config.Address, c.state.String(), c.Config.UseTLS, c.lastPing.String(),
	)
}

func (c *Conn) unsafeGenLogMsg() *log.LogMessage {
	var m *log.LogMessage
	if c.logger != nil {
		m = c.logger.Warn()
	} else {
		m = log.DefaultLogger().Warn()
	}

	return m.
		WithMeta("conn", c.Config.Name).
		WithMeta("peer", c.Config.Address).
		WithMeta("state", c.state.String()).
		WithMeta("tls", c.Config.UseTLS).
		WithMeta("lastPing", c.lastPing.String())
}

func (c *Conn) GenLogMsg() *log.LogMessage {
	c.muConn.RLock()
	defer c.muConn.RUnlock()
	return c.unsafeGenLogMsg()
}

func (c *Conn) Write(b []byte) (int, error) {
	c.muSend.Lock()
	defer c.muSend.Unlock()
	if c.state != ConnStateOpen {
		return 0, ErrConnectionNotEstablished
	}

	if err := c.raw.SetWriteDeadline(time.Now().UTC().Add(c.Config.MessageSendTimeout)); err != nil {
		return 0, err
	}

	i, err := c.raw.Write(b)

	if err := c.raw.SetWriteDeadline(time.Time{}); err != nil {
		return 0, err
	}

	return i, err
}

func (c *Conn) SafeWrite(b []byte) error {
	_, err := c.Write(b)
	return err
}

func (c *Conn) Read(b []byte) (int, error) {
	c.muConn.Lock()
	defer c.muConn.Unlock()
	if c.state != ConnStateOpen {
		return 0, ErrConnectionNotEstablished
	}
	return c.raw.Read(b)
}

func (c *Conn) Register(action Action, fn HandlerFunc) {
	c.muConn.Lock()
	defer c.muConn.Unlock()
	if c.Config.Handlers == nil {
		c.Config.Handlers = make(map[Action]HandlerFunc)
	}
	c.Config.Handlers[action] = fn
}

func (c *Conn) Listen() {
	c.muConn.Lock()
	if c.state == ConnStateOpen {
		return
	}
	c.state = ConnStateOpen
	c.pongCh = make(chan struct{}, 1)
	c.ReadDone = make(chan struct{})

	c.muConn.Unlock()

	c.readLoop()
}

func (c *Conn) Connect() error {
	c.muConn.Lock()
	defer c.muConn.Unlock()

	c.muSend.Lock()
	defer c.muSend.Unlock()

	if c.state == ConnStateReconnecting {
		return nil
	}
	return c.connect()
}

// Internal connection handler
//
// Ensure that the caller holds the lock
func (c *Conn) connect() error {
	if c.state == ConnStateOpen {
		return nil
	}

	c.unsafeGenLogMsg().Info().Msg("connecting").Send()

	conn, err := net.Dial("tcp", c.Config.Address)
	if err != nil {
		c.unsafeGenLogMsg().Error().Msgf("dial failed: %v", err).Send()
		return errors.Join(ErrConnectionNotEstablished, fmt.Errorf("dial failed: %w", err))
	}

	if c.Config.UseTLS {
		conn, err = WrapTLS(conn, c.Config.TLSConfig)
		if err != nil {
			c.unsafeGenLogMsg().Error().Msgf("tls wrap failed: %v", err).Send()
			return errors.Join(ErrConnectionTLSUpgradeFailed, fmt.Errorf("tls wrap failed: %w", err))
		}
	}

	c.unsafeGenLogMsg().Info().Msg("connected").Send()

	c.raw = conn
	c.state = ConnStateOpen
	c.lastPing = time.Now().UTC()

	c.pongCh = make(chan struct{}, 1)
	c.ReadDone = make(chan struct{})

	go c.heartbeatLoop()
	go c.readLoop()
	return nil
}

func (c *Conn) Close() error {
	c.muConn.Lock()
	defer c.muConn.Unlock()

	c.muSend.Lock()
	defer c.muSend.Unlock()

	c.unsafeGenLogMsg().Info().Msg("closing").Send()

	err := c.raw.Close()
	if err != nil {
		c.state = ConnStateUnknown
		c.unsafeGenLogMsg().Error().Msgf("failed to close connection: %v", err).Send()
		return err
	}

	if c.ReadDone != nil {
		select {
		case <-c.ReadDone:
		default:
			close(c.ReadDone)
		}
		c.ReadDone = nil
	}

	c.raw = nil
	c.pongCh = nil
	c.state = ConnStateClosed
	return nil
}

// Internal reconnect handler
//
// Ensure that the caller holds the lock
func (c *Conn) reconnect() error {
	c.muConn.Lock()
	defer c.muConn.Unlock()

	c.muSend.Lock()
	defer c.muSend.Unlock()

	if c.state == ConnStateClosed {
		return ErrConnectionClosed
	}
	c.state = ConnStateReconnecting

	c.unsafeGenLogMsg().Info().Msg("reconnecting").Send()
	return c.connect()
}

func (c *Conn) Reconnect() error {
	c.muConn.Lock()
	if c.state == ConnStateClosed {
		c.muConn.Unlock()
		return ErrConnectionClosed
	}

	if c.state == ConnStateReconnecting {
		c.muConn.Unlock()
		return ErrConnectionAlreadyReconnecting
	}
	c.muConn.Unlock()

	allErrs := make([]error, 0, c.Config.MaxReconnectionAttempts+1)
	allErrs = append(allErrs, ErrExhaustedReconnectAttempts)
	for i := 0; i < c.Config.MaxReconnectionAttempts; i++ {
		err := c.reconnect()
		if err == nil {
			return nil
		}

		allErrs = append(allErrs, err)
		c.GenLogMsg().Debug().
			WithMetaf("attempt", "%d/%d", i, c.Config.MaxReconnectionAttempts).
			Msg("reconnect failed").Send()
		time.Sleep(c.Config.ReconnectionDelay)
	}

	c.GenLogMsg().Warn().
		WithMetaf("attempts", "%d", c.Config.MaxReconnectionAttempts).
		Msg("reconnect failed").Send()
	return errors.Join(allErrs...)
}

func (c *Conn) IsOpen() bool {
	c.muConn.Lock()
	defer c.muConn.Unlock()
	return c.state == ConnStateOpen
}

func (c *Conn) ReconnectOrClose() error {
	var err error
	if c.Config.AutoReconnect {
		err = c.Reconnect()
		if err == nil {
			return nil
		}
	}

	return errors.Join(c.Close(), err)
}

func (c *Conn) readLoop() {
	c.muConn.Lock()
	c.ReadDone = make(chan struct{})
	c.muConn.Unlock()

	c.GenLogMsg().Debug().Msg("starting read loop").Send()

	for {
		if c.state != ConnStateOpen {
			c.GenLogMsg().Debug().Msg("exiting read loop").Send()
			return
		}

		headerBuf := make([]byte, 9)
		if _, err := io.ReadFull(c.raw, headerBuf); err != nil {
			if errors.Is(err, io.EOF) {
				c.GenLogMsg().Info().Msg("connection closed by peer").Send()
				if err := c.Close(); err != nil {
					c.GenLogMsg().Error().Msgf("failed to close connection: %v", err).Send()
				}
				return
			}

			c.GenLogMsg().Error().Msgf("failed to read header: %v", err).Send()
			continue
		}

		header, err := UnmarshalHeader(headerBuf)
		if err != nil {
			c.GenLogMsg().Error().
				WithMetaf("header", "%#v", headerBuf).
				Msgf("failed to unmarshal header: %v", err).Send()
			continue
		}

		handler, ok := c.Config.Handlers[header.Action]
		if !ok {
			c.GenLogMsg().Info().Msgf("no handler for action %d", header.Action).Send()
			continue
		}

		if header.Len > uint64(c.Config.MaxMessageSize) {
			c.GenLogMsg().Info().
				WithMetaf("size", "%d>%d", header.Len, c.Config.MaxMessageSize).
				Msg("payload too large, killing connection").Send()

			if err := c.Close(); err != nil {
				c.GenLogMsg().Error().
					Msgf("failed to close connection: %v", errors.Join(ErrPayloadTooLarge, err)).
					Send()
			}
			return
		}

		payload := make([]byte, header.Len)
		if _, err := io.ReadFull(c.raw, payload); err != nil {
			c.GenLogMsg().Error().Msgf("failed to read payload: %v", err).Send()
			continue
		}

		go handler(c, header, bytes.NewReader(payload))
	}
}

func (c *Conn) heartbeatLoop() {
	if c.Config.HeartbeatInterval == 0 {
		c.GenLogMsg().Debug().Msg("heartbeat interval is 0, skipping heartbeat loop").Send()
		return
	}

	t := time.NewTicker(c.Config.HeartbeatInterval)
	defer t.Stop()
	c.GenLogMsg().Debug().Msg("starting heartbeat loop").Send()

	for range t.C {
		c.muConn.Lock()
		if c.state == ConnStateClosed {
			c.unsafeGenLogMsg().Debug().Msg("exiting heartbeat loop").Send()
			c.muConn.Unlock()
			return
		}
		c.muConn.Unlock()

	drain:
		for {
			select {
			case <-c.pongCh:
			default:
				break drain
			}
		}

		if err := c.sendPing(); err != nil {
			c.GenLogMsg().Error().Msgf("failed to send ping: %v", err).Send()
			go c.ReconnectOrClose()

			return
		}

		select {
		case <-c.pongCh:
			c.muConn.Lock()
			c.lastPing = time.Now().UTC()
			c.muConn.Unlock()
		case <-time.After(10 * time.Second):
			c.GenLogMsg().Warn().Msg("pong timeout").Send()
			go c.ReconnectOrClose()

			return
		}

		time.Sleep(c.Config.HeartbeatInterval)
	}
}

// Internal ping handler
func (c *Conn) sendPing() error {
	h := Header{Action: ActionPing, Len: 0}
	b, err := h.MarshalBytes()
	if err != nil {
		return err
	}

	err = c.SafeWrite(b)
	c.GenLogMsg().Debug().Msg("sent ping").Send()
	return err
}

func (c *Conn) sendPong() error {
	h := Header{Action: ActionPong, Len: 0}
	b, err := h.MarshalBytes()
	if err != nil {
		return err
	}

	err = c.SafeWrite(b)
	c.GenLogMsg().Debug().Msg("sent pong").Send()
	return err
}
