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
	c := &protocolBase{
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
	c.activeReader = c.netReadWriter
	c.activeWriter = c.netReadWriter
	c.sendChan = make(chan []byte)
	go c.writer()
	return c
}

func (c *protocolBase) Terminate() {
	if c.terminated == true {
		return
	}
	c.terminated = true
	c.netReadWriter.Close()

	if c.syncMode == false {
		close(c.sendChan)
		<-c.writerJoined
	}
}

func (c *protocolBase) SetSyncMode(val bool) {
	if val == c.syncMode {
		return
	}
	c.syncMode = val

	if val == true {
		close(c.sendChan)
		<-c.writerJoined

	} else {
		c.sendChan = make(chan []byte)
		go c.writer()
	}
}

func (c *protocolBase) SetReadBinary(val bool) {
	if val == c.readBinary {
		return
	}
	c.readBinary = val
}

func (c *protocolBase) SetReadCompressionOn() error {
	if c.activeReader == c.zlibReader {
		return fmt.Errorf("zlib already activated")
	}

	if c.zlibReader == nil {
		var err error
		c.zlibReader, err = zlib.NewReader(c.netReadWriter)
		if err != nil {
			panic(err)
		}
	} else {
		c.zlibReader.(zlib.Resetter).Reset(c.netReadWriter, nil)
	}
	c.activeReader = c.zlibReader

	dolog(LevelDebug, "[read zlib on]")
	return nil
}

func (c *protocolBase) SetWriteCompression(val bool) {
	if (val && c.activeWriter == c.zlibWriter) ||
		(!val && c.activeWriter != c.zlibWriter) {
		return
	}

	if val == true {
		c.zlibWriter = zlib.NewWriter(c.netReadWriter)
		c.activeWriter = c.zlibWriter
		dolog(LevelDebug, "[write zlib on]")
	} else {
		c.zlibWriter.Close()
		c.activeWriter = c.netReadWriter
		dolog(LevelDebug, "[write zlib off]")
	}
}

func (c *protocolBase) ReadMessage() (string, error) {
	// Terminate() was called in a previous run
	if c.terminated == true {
		return "", errorTerminated
	}

	for {
		msg, err := readUntilDelim(c.activeReader, c.msgDelim)
		if err != nil {
			// zlib EOF: disable and read again
			if c.activeReader == c.zlibReader && err == io.EOF {
				dolog(LevelDebug, "[read zlib off]")
				c.zlibReader.Close()
				c.activeReader = c.netReadWriter
				continue
			}
			if c.terminated == true {
				return "", errorTerminated
			}
			return "", err
		}
		return msg, nil
	}
}

func (c *protocolBase) ReadBinary() ([]byte, error) {
	// Terminate() was called in a previous run
	if c.terminated == true {
		return nil, errorTerminated
	}

	var buf [2048]byte
	for {
		read, err := c.activeReader.Read(buf[:])
		if read == 0 {
			// zlib EOF: disable and read again
			if c.activeReader == c.zlibReader && err == io.EOF {
				dolog(LevelDebug, "[read zlib off]")
				c.zlibReader.Close()
				c.activeReader = c.netReadWriter
				continue
			}
			if c.terminated == true {
				return nil, errorTerminated
			}
			return nil, err
		}
		return buf[:read], nil
	}
}

func (c *protocolBase) writer() {
	for buf := range c.sendChan {
		// do not handle errors here
		c.WriteSync(buf)
	}
	c.writerJoined <- struct{}{}
}

func (c *protocolBase) WriteSync(in []byte) error {
	_, err := c.activeWriter.Write(in)
	return err
}

type msgBinary struct {
	Content []byte
}
