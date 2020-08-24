package proto

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/aler9/go-dc/lineproto"

	"github.com/aler9/dctoolkit/log"
)

const (
	_CONN_READ_TIMEOUT  = 60 * time.Second
	_CONN_WRITE_TIMEOUT = 10 * time.Second
	_MAX_MESSAGE_SIZE   = 10 * 1024
)

const reStrNick = "[^\\$ \\|\n]+"
const reStrAddress = "[a-z0-9\\.-_]+"
const ReStrIp = "[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}"
const reStrPort = "[0-9]{1,5}"

var ErrorTerminated = fmt.Errorf("terminated")

type MsgDecodable interface{}
type MsgEncodable interface{}

type monitoredConnIntf interface {
	PullReadCounter() uint
	PullWriteCounter() uint
}

type Protocol interface {
	Close() error
	SetSyncMode(val bool)
	SetReadBinary(val bool)
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

// timedConn forces net.Conn to use timeouts.
type timedConn struct {
	io.Closer
	conn         net.Conn
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func newTimedConn(conn net.Conn, readTimeout time.Duration,
	writeTimeout time.Duration) io.ReadWriteCloser {
	return &timedConn{
		Closer:       conn,
		conn:         conn,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

func (c *timedConn) Read(buf []byte) (int, error) {
	if c.readTimeout > 0 {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.readTimeout)); err != nil {
			return 0, err
		}
	}
	return c.conn.Read(buf)
}

func (c *timedConn) Write(buf []byte) (int, error) {
	if c.writeTimeout > 0 {
		if err := c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return 0, err
		}
	}
	return c.conn.Write(buf)
}

// monitoredConn implements a read and a writer counter, that provides the
// connection speed.
type monitoredConn struct {
	io.Closer
	in           io.ReadWriteCloser
	readCounter  uint
	writeCounter uint
}

func newMonitoredConn(in io.ReadWriteCloser) *monitoredConn {
	return &monitoredConn{
		Closer: in,
		in:     in,
	}
}

func (c *monitoredConn) Read(buf []byte) (int, error) {
	n, err := c.in.Read(buf)
	c.readCounter += uint(n)
	return n, err
}

func (c *monitoredConn) Write(buf []byte) (int, error) {
	n, err := c.in.Write(buf)
	c.writeCounter += uint(n)
	return n, err
}

func (c *monitoredConn) PullReadCounter() uint {
	ret := c.readCounter
	c.readCounter = 0
	return ret
}

func (c *monitoredConn) PullWriteCounter() uint {
	ret := c.writeCounter
	c.writeCounter = 0
	return ret
}

type ProtocolBase struct {
	logLevel    log.Level
	remoteLabel string
	terminated  uint32 // atomic
	msgDelim    byte
	sendChan    chan []byte
	closer      io.Closer
	monitoredConnIntf
	reader       *lineproto.Reader
	writer       *lineproto.Writer
	readBinary   bool
	syncMode     bool
	writerJoined chan struct{}
}

func newProtocolBase(logLevel log.Level, remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool, msgDelim byte) *ProtocolBase {

	readTimeout := func() time.Duration {
		if applyReadTimeout == true {
			return _CONN_READ_TIMEOUT
		}
		return 0
	}()
	writeTimeout := func() time.Duration {
		if applyWriteTimeout == true {
			return _CONN_WRITE_TIMEOUT
		}
		return 0
	}()

	tc := newTimedConn(nconn, readTimeout, writeTimeout)
	mc := newMonitoredConn(tc)
	rdr := lineproto.NewReader(mc, msgDelim)
	wri := lineproto.NewWriter(mc)

	p := &ProtocolBase{
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
	go p.writeReceiver()
	return p
}

func (p *ProtocolBase) isTerminated() bool {
	return atomic.LoadUint32(&p.terminated) != 0
}

func (p *ProtocolBase) Close() error {
	if !atomic.CompareAndSwapUint32(&p.terminated, 0, 1) {
		return nil // already closing
	}
	p.closer.Close()

	if p.syncMode == false {
		close(p.sendChan)
		<-p.writerJoined
	}
	return nil
}

func (p *ProtocolBase) SetSyncMode(val bool) {
	if val == p.syncMode {
		return
	}
	p.syncMode = val

	if val == true {
		close(p.sendChan)
		<-p.writerJoined

	} else {
		p.sendChan = make(chan []byte)
		go p.writeReceiver()
	}
}

func (p *ProtocolBase) SetReadBinary(val bool) {
	if val == p.readBinary {
		return
	}
	p.readBinary = val
}

func (p *ProtocolBase) ReadMessage() (string, error) {
	// Close() was called in a previous run
	if p.isTerminated() {
		return "", ErrorTerminated
	}

	msg, err := p.reader.ReadLine()
	if err != nil {
		if p.isTerminated() {
			return "", ErrorTerminated
		}
		return "", err
	}
	return string(msg[:len(msg)-1]), nil
}

func (p *ProtocolBase) ReadBinary() ([]byte, error) {
	// Close() was called in a previous run
	if p.isTerminated() {
		return nil, ErrorTerminated
	}

	// TODO: move buf out or make static
	var buf [2048]byte
	read, err := p.reader.Read(buf[:])
	if read == 0 {
		if p.isTerminated() {
			return nil, ErrorTerminated
		}
		return nil, err
	}
	return buf[:read], nil
}

func (p *ProtocolBase) writeReceiver() {
	for buf := range p.sendChan {
		// do not handle errors here
		p.WriteSync(buf)
	}
	p.writerJoined <- struct{}{}
}

func (p *ProtocolBase) WriteSync(in []byte) error {
	err := p.writer.WriteLine(in)
	if err != nil {
		return err
	}
	return p.writer.Flush()
}

func (p *ProtocolBase) Write(in []byte) {
	if p.isTerminated() {
		return
	}
	p.sendChan <- in
}

func (p *ProtocolBase) ReaderEnableZlib() error {
	return p.reader.EnableZlib()
}

func (p *ProtocolBase) WriterEnableZlib() {
	p.writer.EnableZlib()
}

func (p *ProtocolBase) WriterDisableZlib() {
	p.writer.DisableZlib()
}
