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

type protocol interface {
	Terminate()
	SetSyncMode(val bool)
	SetReadBinary(val bool)
	SetReadCompressionOn() error
	SetWriteCompression(val bool)
	Read() (msgDecodable, error)
	Write(msg msgEncodable)
	WriteSync(in []byte) error
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
	zlibReader    io.ReadCloser
	zlibWriter    io.WriteCloser
	currentReader io.Reader
	currentWriter io.Writer
	writerJoined  chan struct{}
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
	rbc := newReadBufferedConn(tc)

	p := &protocolBase{
		remoteLabel:   remoteLabel,
		msgDelim:      msgDelim,
		writerJoined:  make(chan struct{}),
		readBinary:    false,
		netReadWriter: rbc,
	}
	p.currentReader = p.netReadWriter
	p.currentWriter = p.netReadWriter
	p.sendChan = make(chan []byte)
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

func (p *protocolBase) SetReadCompressionOn() error {
	if p.currentReader == p.zlibReader {
		return fmt.Errorf("zlib already activated")
	}
	dolog(LevelDebug, "[read zlib on]")

	var err error
	if p.zlibReader == nil {
		p.zlibReader, err = zlib.NewReader(p.netReadWriter)
	} else {
		err = p.zlibReader.(zlib.Resetter).Reset(p.netReadWriter, nil)
	}
	if err != nil {
		return err
	}
	p.currentReader = p.zlibReader
	return nil
}

func (p *protocolBase) SetWriteCompression(val bool) {
	if (val && p.currentWriter == p.zlibWriter) ||
		(!val && p.currentWriter != p.zlibWriter) {
		return
	}

	if val == true {
		dolog(LevelDebug, "[write zlib on]")
		p.zlibWriter = zlib.NewWriter(p.netReadWriter)
		p.currentWriter = p.zlibWriter
	} else {
		dolog(LevelDebug, "[write zlib off]")
		p.zlibWriter.Close()
		p.currentWriter = p.netReadWriter
	}
}

func (p *protocolBase) ReadMessage() (string, error) {
	// Terminate() was called in a previous run
	if p.terminated == true {
		return "", errorTerminated
	}

	for {
		msg, err := readUntilDelim(p.currentReader, p.msgDelim)
		if err != nil {
			// zlib EOF: disable and read again
			if p.currentReader == p.zlibReader && err == io.EOF {
				dolog(LevelDebug, "[read zlib off]")
				p.zlibReader.Close()
				p.currentReader = p.netReadWriter
				continue
			}
			if p.terminated == true {
				return "", errorTerminated
			}
			return "", err
		}
		return msg, nil
	}
}

func (p *protocolBase) ReadBinary() ([]byte, error) {
	// Terminate() was called in a previous run
	if p.terminated == true {
		return nil, errorTerminated
	}

	var buf [2048]byte
	for {
		read, err := p.currentReader.Read(buf[:])
		if read == 0 {
			// zlib EOF: disable and read again
			if p.currentReader == p.zlibReader && err == io.EOF {
				dolog(LevelDebug, "[read zlib off]")
				p.zlibReader.Close()
				p.currentReader = p.netReadWriter
				continue
			}
			if p.terminated == true {
				return nil, errorTerminated
			}
			return nil, err
		}
		return buf[:read], nil
	}
}

func (p *protocolBase) writer() {
	for buf := range p.sendChan {
		// do not handle errors here
		p.WriteSync(buf)
	}
	p.writerJoined <- struct{}{}
}

func (p *protocolBase) WriteSync(in []byte) error {
	_, err := p.currentWriter.Write(in)
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
