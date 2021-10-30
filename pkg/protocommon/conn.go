package protocommon

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/aler9/go-dc/lineproto"

	"github.com/aler9/dctk/pkg/log"
)

const (
	connReadTimeout  = 60 * time.Second
	connWriteTimeout = 10 * time.Second
)

// ReStrNick is the regex to parse a nickname.
const ReStrNick = "[^\\$ \\|\n]+"

// ReStrIP is the regex to parse an IPv4.
const ReStrIP = "[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}"

// ReStrPort is the regex to parse a port.
const ReStrPort = "[0-9]{1,5}"

// ErrorTerminated is raised when the connection is terminated.
var ErrorTerminated = fmt.Errorf("terminated")

// MsgDecodable is implemented by all decodable messages.
type MsgDecodable interface{}

// MsgEncodable  is implemented by all encodable messages.
type MsgEncodable interface{}

type monitoredConnIntf interface {
	PullReadCounter() uint
	PullWriteCounter() uint
}

// MsgBinary is a binary message.
type MsgBinary struct {
	Content []byte
}

// BaseConn is the base connection used by DC protocols.
type BaseConn struct {
	logLevel    log.Level
	remoteLabel string
	terminated  uint32 // atomic
	msgDelim    byte
	sendChan    chan []byte
	closer      io.Closer
	monitoredConnIntf
	reader       *lineproto.Reader
	writer       *lineproto.Writer
	binaryMode   bool
	syncMode     bool
	writerJoined chan struct{}
}

// NewBaseConn allocates a BaseConn.
func NewBaseConn(logLevel log.Level,
	remoteLabel string,
	nconn net.Conn,
	applyReadTimeout bool,
	applyWriteTimeout bool,
	msgDelim byte) *BaseConn {
	readTimeout := func() time.Duration {
		if applyReadTimeout {
			return connReadTimeout
		}
		return 0
	}()
	writeTimeout := func() time.Duration {
		if applyWriteTimeout {
			return connWriteTimeout
		}
		return 0
	}()

	tc := newTimedConn(nconn, readTimeout, writeTimeout)
	mc := newMonitoredConn(tc)
	rdr := lineproto.NewReader(mc, msgDelim)
	wri := lineproto.NewWriter(mc)

	c := &BaseConn{
		logLevel:          logLevel,
		remoteLabel:       remoteLabel,
		msgDelim:          msgDelim,
		writerJoined:      make(chan struct{}),
		closer:            mc,
		monitoredConnIntf: mc,
		reader:            rdr,
		writer:            wri,
		sendChan:          make(chan []byte),
	}

	go c.writeReceiver()

	return c
}

// Close closes the connection.
func (c *BaseConn) Close() error {
	if !atomic.CompareAndSwapUint32(&c.terminated, 0, 1) {
		return nil // already closing
	}
	c.closer.Close()

	if !c.syncMode {
		close(c.sendChan)
		<-c.writerJoined
	}
	return nil
}

func (c *BaseConn) isTerminated() bool {
	return atomic.LoadUint32(&c.terminated) != 0
}

// LogLevel returns the log level.
func (c *BaseConn) LogLevel() log.Level {
	return c.logLevel
}

// RemoteLabel returns the remote label.
func (c *BaseConn) RemoteLabel() string {
	return c.remoteLabel
}

// BinaryMode returns the binary mode.
func (c *BaseConn) BinaryMode() bool {
	return c.binaryMode
}

// SetSyncMode sets the sync mode.
func (c *BaseConn) SetSyncMode(val bool) {
	if val == c.syncMode {
		return
	}
	c.syncMode = val

	if val {
		close(c.sendChan)
		<-c.writerJoined
	} else {
		c.sendChan = make(chan []byte)
		go c.writeReceiver()
	}
}

// SetBinaryMode sets the binary mode.
func (c *BaseConn) SetBinaryMode(val bool) {
	c.binaryMode = val
}

// ReadMessage reads a message.
func (c *BaseConn) ReadMessage() (string, error) {
	// Close() was called in a previous run
	if c.isTerminated() {
		return "", ErrorTerminated
	}

	msg, err := c.reader.ReadLine()
	if err != nil {
		if c.isTerminated() {
			return "", ErrorTerminated
		}
		return "", err
	}
	return string(msg[:len(msg)-1]), nil
}

// ReadBinary reads binary data.
func (c *BaseConn) ReadBinary() ([]byte, error) {
	// Close() was called in a previous run
	if c.isTerminated() {
		return nil, ErrorTerminated
	}

	// TODO: move buf out or make static
	var buf [2048]byte
	read, err := c.reader.Read(buf[:])
	if read == 0 {
		if c.isTerminated() {
			return nil, ErrorTerminated
		}
		return nil, err
	}
	return buf[:read], nil
}

func (c *BaseConn) writeReceiver() {
	for buf := range c.sendChan {
		// do not handle errors here
		c.WriteSync(buf)
	}
	c.writerJoined <- struct{}{}
}

// WriteSync writes a message in sync mode.
func (c *BaseConn) WriteSync(in []byte) error {
	err := c.writer.WriteLine(in)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// Write writes a message in asynchronous mode.
func (c *BaseConn) Write(in []byte) {
	if c.isTerminated() {
		return
	}
	c.sendChan <- in
}

// EnableReaderZlib enables zlib on readings.
func (c *BaseConn) EnableReaderZlib() error {
	return c.reader.EnableZlib()
}

// EnableWriterZlib enables zlib on writings.
func (c *BaseConn) EnableWriterZlib() error {
	return c.writer.EnableZlib()
}

// DisableWriterZlib disables zlib on writings.
func (c *BaseConn) DisableWriterZlib() error {
	return c.writer.DisableZlib()
}
