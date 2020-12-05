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

const ReStrNick = "[^\\$ \\|\n]+"
const ReStrIP = "[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}"
const ReStrPort = "[0-9]{1,5}"

var ErrorTerminated = fmt.Errorf("terminated")

type MsgDecodable interface{}
type MsgEncodable interface{}

type monitoredConnIntf interface {
	PullReadCounter() uint
	PullWriteCounter() uint
}

type Conn interface {
	Close() error
	SetSyncMode(val bool)
	SetBinaryMode(val bool)
	Read() (MsgDecodable, error)
	Write(msg MsgEncodable)
	WriteSync(in []byte) error
	monitoredConnIntf
	ReaderEnableZlib() error
	WriterEnableZlib()
	WriterDisableZlib()
}

type MsgBinary struct {
	Content []byte
}

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

func NewBaseConn(logLevel log.Level, remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool, msgDelim byte) *BaseConn {

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

func (c *BaseConn) isTerminated() bool {
	return atomic.LoadUint32(&c.terminated) != 0
}

func (c *BaseConn) LogLevel() log.Level {
	return c.logLevel
}

func (c *BaseConn) RemoteLabel() string {
	return c.remoteLabel
}

func (c *BaseConn) BinaryMode() bool {
	return c.binaryMode
}

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

func (c *BaseConn) SetBinaryMode(val bool) {
	c.binaryMode = val
}

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

func (c *BaseConn) WriteSync(in []byte) error {
	err := c.writer.WriteLine(in)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *BaseConn) Write(in []byte) {
	if c.isTerminated() {
		return
	}
	c.sendChan <- in
}

func (c *BaseConn) ReaderEnableZlib() error {
	return c.reader.EnableZlib()
}

func (c *BaseConn) WriterEnableZlib() {
	c.writer.EnableZlib()
}

func (c *BaseConn) WriterDisableZlib() {
	c.writer.DisableZlib()
}
