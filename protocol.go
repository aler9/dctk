package dctoolkit

import (
	"bufio"
	"compress/zlib"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	_CONN_READ_TIMEOUT  = 60 * time.Second
	_CONN_WRITE_TIMEOUT = 10 * time.Second
	_MAX_MESSAGE_SIZE   = 10 * 1024
)

type msgDecodable interface{}
type msgEncodable interface{}

type monitoredConnIntf interface {
	PullReadCounter() uint
	PullWriteCounter() uint
}

type zlibSwitchableConnIntf interface {
	SetReadCompressionTrue() error
	SetWriteCompression(val bool)
}

type protocol interface {
	Terminate()
	SetSyncMode(val bool)
	SetReadBinary(val bool)
	Read() (msgDecodable, error)
	Write(msg msgEncodable)
	WriteSync(in []byte) error
	monitoredConnIntf
	zlibSwitchableConnIntf
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

// monitoredConn implements a read and a writer counter, that is used to
// compute the connection speed.
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

// readBufferedConn buffer the readings. This is done for two reasons:
// 1) in this way SetReadDeadline() is called only when buffer is refilled, and
//    not every time Read() or ReadByte() is called, improving efficiency;
// 2) we must provide a io.ByteReader interface to zlib.NewReader(), otherwise
//    it automatically adds a bufio layer that messes up the zlib on/off phase.
type readBufferedConn struct {
	io.WriteCloser
	*bufio.Reader
}

func newReadBufferedConn(in io.ReadWriteCloser) io.ReadWriteCloser {
	return &readBufferedConn{
		WriteCloser: in,
		Reader:      bufio.NewReaderSize(in, 2048), // TCP MTU is 1460 bytes
	}
}

// zlibSwitchableConn implements a read/write compression that can be switched
// on or off at any time.
type zlibSwitchableConn struct {
	in           io.ReadWriteCloser
	zlibReader   io.ReadCloser
	zlibWriter   io.WriteCloser
	activeReader io.Reader
	activeWriter io.Writer
}

func newZlibSwitchableConn(in io.ReadWriteCloser) *zlibSwitchableConn {
	return &zlibSwitchableConn{
		in:           in,
		activeReader: in,
		activeWriter: in,
	}
}

func (c *zlibSwitchableConn) Close() error {
	if c.activeReader == c.zlibReader {
		c.zlibReader.Close()
	}
	if c.activeWriter == c.zlibWriter {
		c.zlibWriter.Close()
	}
	return c.in.Close()
}

func (c *zlibSwitchableConn) Read(buf []byte) (int, error) {
	for {
		n, err := c.activeReader.Read(buf)

		// zlib EOF: disable and read again
		if n == 0 && err == io.EOF && c.activeReader == c.zlibReader {
			dolog(LevelDebug, "[read zlib off]")
			c.zlibReader.Close()
			c.activeReader = c.in
			continue
		}
		return n, err
	}
}

func (c *zlibSwitchableConn) Write(buf []byte) (int, error) {
	return c.activeWriter.Write(buf)
}

func (c *zlibSwitchableConn) SetReadCompressionTrue() error {
	if c.activeReader == c.zlibReader {
		return fmt.Errorf("zlib already activated")
	}
	dolog(LevelDebug, "[read zlib on]")

	var err error
	if c.zlibReader == nil {
		c.zlibReader, err = zlib.NewReader(c.in)
	} else {
		err = c.zlibReader.(zlib.Resetter).Reset(c.in, nil)
	}
	if err != nil {
		return err
	}
	c.activeReader = c.zlibReader
	return nil
}

func (c *zlibSwitchableConn) SetWriteCompression(val bool) {
	if (val && c.activeWriter == c.zlibWriter) ||
		(!val && c.activeWriter != c.zlibWriter) {
		return
	}

	if val == true {
		dolog(LevelDebug, "[write zlib on]")
		if c.zlibWriter == nil {
			c.zlibWriter = zlib.NewWriter(c.in)
		} else {
			c.zlibWriter.(*zlib.Writer).Reset(c.in)
		}
		c.activeWriter = c.zlibWriter
	} else {
		dolog(LevelDebug, "[write zlib off]")
		c.zlibWriter.Close()
		c.activeWriter = c.in
	}
}

// this is like bufio.ReadSlice(), except it does not buffer
// anything, to allow the zlib on/off phase
// and it also strips the delimiter
func readUntilDelim(in io.Reader, delim byte) (string, error) {
	var buffer [_MAX_MESSAGE_SIZE]byte
	offset := 0
	for {
		// read one character at a time
		read, err := in.Read(buffer[offset : offset+1])
		if read == 0 {
			return "", err
		}
		offset++

		if buffer[offset-1] == delim {
			return string(buffer[:offset-1]), nil
		}

		if offset >= len(buffer) {
			return "", fmt.Errorf("message buffer exhausted")
		}
	}
}

type protocolBase struct {
	remoteLabel   string
	msgDelim      byte
	sendChan      chan []byte
	terminated    bool
	readBinary    bool
	syncMode      bool
	netReadWriter io.ReadWriteCloser
	writerJoined  chan struct{}
	monitoredConnIntf
	zlibSwitchableConnIntf
}

func newProtocolBase(remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool, msgDelim byte) *protocolBase {
	tc := newTimedConn(nconn,
		func() time.Duration {
			if applyReadTimeout == true {
				return _CONN_READ_TIMEOUT
			}
			return 0
		}(),
		func() time.Duration {
			if applyWriteTimeout == true {
				return _CONN_WRITE_TIMEOUT
			}
			return 0
		}())
	mc := newMonitoredConn(tc)
	rbc := newReadBufferedConn(mc)
	zsc := newZlibSwitchableConn(rbc)

	p := &protocolBase{
		remoteLabel:   remoteLabel,
		msgDelim:      msgDelim,
		writerJoined:  make(chan struct{}),
		readBinary:    false,
		netReadWriter: zsc,
	}
	p.sendChan = make(chan []byte)
	p.monitoredConnIntf = mc
	p.zlibSwitchableConnIntf = zsc
	go p.writer()
	return p
}

func (p *protocolBase) Terminate() {
	if p.terminated == true {
		return
	}
	p.terminated = true
	p.netReadWriter.Close()

	if p.syncMode == false {
		close(p.sendChan)
		<-p.writerJoined
	}
}

func (p *protocolBase) SetSyncMode(val bool) {
	if val == p.syncMode {
		return
	}
	p.syncMode = val

	if val == true {
		close(p.sendChan)
		<-p.writerJoined

	} else {
		p.sendChan = make(chan []byte)
		go p.writer()
	}
}

func (p *protocolBase) SetReadBinary(val bool) {
	if val == p.readBinary {
		return
	}
	p.readBinary = val
}

func (p *protocolBase) ReadMessage() (string, error) {
	// Terminate() was called in a previous run
	if p.terminated == true {
		return "", errorTerminated
	}

	msg, err := readUntilDelim(p.netReadWriter, p.msgDelim)
	if err != nil {
		if p.terminated == true {
			return "", errorTerminated
		}
		return "", err
	}
	return msg, nil
}

func (p *protocolBase) ReadBinary() ([]byte, error) {
	// Terminate() was called in a previous run
	if p.terminated == true {
		return nil, errorTerminated
	}

	var buf [2048]byte
	read, err := p.netReadWriter.Read(buf[:])
	if read == 0 {
		if p.terminated == true {
			return nil, errorTerminated
		}
		return nil, err
	}
	return buf[:read], nil
}

func (p *protocolBase) writer() {
	for buf := range p.sendChan {
		// do not handle errors here
		p.WriteSync(buf)
	}
	p.writerJoined <- struct{}{}
}

func (p *protocolBase) WriteSync(in []byte) error {
	_, err := p.netReadWriter.Write(in)
	return err
}

func (p *protocolBase) Write(in []byte) {
	if p.terminated == true {
		return
	}
	p.sendChan <- in
}

type msgBinary struct {
	Content []byte
}
