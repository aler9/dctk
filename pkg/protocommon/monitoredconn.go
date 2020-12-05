package protocommon

import (
	"io"
)

// monitoredConn implements a read and a writer counter, that computes the
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
