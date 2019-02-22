package dctoolkit

import (
	"bufio"
	"compress/zlib"
	"fmt"
	"io"
	"net"
	"time"
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

// this forces net.Conn to use timeouts
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

// we buffer the readings for two reasons:
// 1) in this way SetReadDeadline() is called only when buffer is refilled, and
//    not every time ReadByte() is called, improving efficiency;
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
	var buffer [10 * 1024]byte // max message size
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
	activeReader  io.Reader
	zlibWriter    *zlib.Writer
	activeWriter  io.Writer
	writerJoined  chan struct{}
}

func newProtocolBase(remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool, msgDelim byte) *protocolBase {
	p := &protocolBase{
		remoteLabel:  remoteLabel,
		msgDelim:     msgDelim,
		writerJoined: make(chan struct{}),
		readBinary:   false,
		netReadWriter: newReadBufferedConn(newTimedConn(nconn,
			func() time.Duration {
				if applyReadTimeout == true {
					return 60 * time.Second
				}
				return 0
			}(),
			func() time.Duration {
				if applyWriteTimeout == true {
					return 10 * time.Second
				}
				return 0
			}())),
	}
	p.activeReader = p.netReadWriter
	p.activeWriter = p.netReadWriter
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
	if p.activeReader == p.zlibReader {
		return fmt.Errorf("zlib already activated")
	}

	if p.zlibReader == nil {
		var err error
		p.zlibReader, err = zlib.NewReader(p.netReadWriter)
		if err != nil {
			panic(err)
		}
	} else {
		p.zlibReader.(zlib.Resetter).Reset(p.netReadWriter, nil)
	}
	p.activeReader = p.zlibReader

	dolog(LevelDebug, "[read zlib on]")
	return nil
}

func (p *protocolBase) SetWriteCompression(val bool) {
	if (val && p.activeWriter == p.zlibWriter) ||
		(!val && p.activeWriter != p.zlibWriter) {
		return
	}

	if val == true {
		p.zlibWriter = zlib.NewWriter(p.netReadWriter)
		p.activeWriter = p.zlibWriter
		dolog(LevelDebug, "[write zlib on]")
	} else {
		p.zlibWriter.Close()
		p.activeWriter = p.netReadWriter
		dolog(LevelDebug, "[write zlib off]")
	}
}

func (p *protocolBase) ReadMessage() (string, error) {
	// Terminate() was called in a previous run
	if p.terminated == true {
		return "", errorTerminated
	}

	for {
		msg, err := readUntilDelim(p.activeReader, p.msgDelim)
		if err != nil {
			// zlib EOF: disable and read again
			if p.activeReader == p.zlibReader && err == io.EOF {
				dolog(LevelDebug, "[read zlib off]")
				p.zlibReader.Close()
				p.activeReader = p.netReadWriter
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
		read, err := p.activeReader.Read(buf[:])
		if read == 0 {
			// zlib EOF: disable and read again
			if p.activeReader == p.zlibReader && err == io.EOF {
				dolog(LevelDebug, "[read zlib off]")
				p.zlibReader.Close()
				p.activeReader = p.netReadWriter
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
	_, err := p.activeWriter.Write(in)
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
