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

	"github.com/lattesec/ctfjx/pkg/log"
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

func (c *Conn) unsafeLogf(format string, v ...any) string {
	return fmt.Sprintf("%s %s", c.unsafeString(), fmt.Sprintf(format, v...))
}

func (c *Conn) Logf(format string, v ...any) string {
	c.muConn.RLock()
	defer c.muConn.RUnlock()
	return c.unsafeLogf(format, v...)
}

func (c *Conn) Write(b []byte) (int, error) {
	c.muSend.Lock()
	defer c.muSend.Unlock()
	if c.state != ConnStateOpen {
		return 0, ErrConnectionNotEstablished
	}
	c.raw.SetWriteDeadline(time.Now().UTC().Add(c.Config.MessageSendTimeout))
	i, err := c.raw.Write(b)
	c.raw.SetWriteDeadline(time.Time{})
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

func (c *Conn) Listen() error {
	c.muConn.Lock()
	if c.state == ConnStateOpen {
		return nil
	}
	c.state = ConnStateOpen
	c.pongCh = make(chan struct{}, 1)
	c.ReadDone = make(chan struct{})

	c.muConn.Unlock()

	return c.readLoop()
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

	log.Debugln(c.unsafeLogf("connecting to %s", c.Config.Address))

	conn, err := net.Dial("tcp", c.Config.Address)
	if err != nil {
		log.Debugln(c.unsafeLogf("dial %s failed: %v", c.Config.Address, err))
		return errors.Join(ErrConnectionNotEstablished, fmt.Errorf("dial failed: %w", err))
	}

	if c.Config.UseTLS {
		conn, err = WrapTLS(conn, c.Config.TLSConfig)
		if err != nil {
			log.Debugln(c.unsafeLogf("tls wrap failed: %v", err))
			return errors.Join(ErrConnectionTLSUpgradeFailed, fmt.Errorf("tls wrap failed: %w", err))
		}
	}

	log.Debugln(c.unsafeLogf("connected"))

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

	log.Debugln(c.unsafeLogf("closing connection"))

	err := c.raw.Close()
	if err != nil {
		c.state = ConnStateUnknown
		log.Errorln(c.unsafeLogf("failed to close connection: %v", err))
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

	log.Debugln(c.unsafeLogf("reconnecting"))
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
		log.Debugln(c.Logf("reconnect [%d of %d] failed", i, c.Config.MaxReconnectionAttempts))
		time.Sleep(c.Config.ReconnectionDelay)
	}

	log.Debugln(c.Logf("reconnect failed after %d attempts", c.Config.MaxReconnectionAttempts))
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

func (c *Conn) readLoop() error {
	c.muConn.Lock()
	c.ReadDone = make(chan struct{})
	c.muConn.Unlock()

	log.Debugln(c.Logf("starting read loop"))

	for {
		if c.state != ConnStateOpen {
			log.Debugln(c.Logf("connection not open, exiting read loop"))
			return nil
		}

		headerBuf := make([]byte, 9)
		if _, err := io.ReadFull(c.raw, headerBuf); err != nil {
			if errors.Is(err, io.EOF) {
				log.Infoln(c.Logf("connection closed by peer"))
				return c.Close()
			}

			log.Errorln(c.Logf("failed to read header: %v", err))
			continue
		}
		log.Debugln(c.Logf("recv header buf: %v", headerBuf))

		header, err := UnmarshalHeader(headerBuf)
		if err != nil {
			log.Errorln(c.Logf("failed to unmarshal header: %v", err))
			continue
		}

		handler, ok := c.Config.Handlers[header.Action]
		log.Debugln(c.Logf("handlers: %v", c.Config.Handlers))
		if !ok {
			log.Errorln(c.Logf("no handler for action %d", header.Action))
			continue
		}

		if header.Len > uint64(c.Config.MaxMessageSize) {
			log.Errorln(c.Logf("payload too large, killing connection: %d > %d", header.Len, c.Config.MaxMessageSize))
			c.Close()
			return ErrPayloadTooLarge
		}

		payload := make([]byte, header.Len)
		if _, err := io.ReadFull(c.raw, payload); err != nil {
			log.Errorln(c.Logf("failed to read payload: %v", err))
			continue
		}

		go handler(c, header, bytes.NewReader(payload))
	}
}

func (c *Conn) heartbeatLoop() {
	if c.Config.HeartbeatInterval == 0 {
		log.Debugln(c.Logf("heartbeat loop not started"))
		return
	}

	t := time.NewTicker(c.Config.HeartbeatInterval)
	defer t.Stop()
	log.Debugln(c.Logf("starting heartbeat loop"))

	for range t.C {
		c.muConn.Lock()
		if c.state == ConnStateClosed {
			log.Debugln(c.unsafeLogf("connection closed, exiting heartbeat loop"))
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
			log.Debugln(c.Logf("failed to send ping: %v", err))
			go c.ReconnectOrClose()

			return
		}

		select {
		case <-c.pongCh:
			c.muConn.Lock()
			c.lastPing = time.Now().UTC()
			c.muConn.Unlock()
		case <-time.After(10 * time.Second):
			log.Debugln(c.Logf("pong timeout"))
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
	log.Debugln(c.Logf("sent ping"))
	return err
}

func (c *Conn) sendPong() error {
	h := Header{Action: ActionPong, Len: 0}
	b, err := h.MarshalBytes()
	if err != nil {
		return err
	}

	err = c.SafeWrite(b)
	log.Debugln(c.Logf("sent pong"))
	return err
}
