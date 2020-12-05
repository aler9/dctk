package protocommon

import (
	"io"
	"net"
	"time"
)

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
