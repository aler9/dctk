package dctoolkit

import (
	"encoding/base32"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aler9/dctoolkit/tiger"
)

var dirTTH = tiger.HashMust("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

var errorArgsFormat = fmt.Errorf("not formatted correctly")

// base32 without padding, which can be one or multiple =
func dcBase32Decode(in string) []byte {
	// add missing padding
	if (len(in) % 8) != 0 {
		mlen := (8 - (len(in) % 8))
		for n := 0; n < mlen; n++ {
			in += "="
		}
	}
	out, _ := base32.StdEncoding.DecodeString(in)
	return out
}

func dcReadableQuery(request string) string {
	if strings.HasPrefix(request, "tthl TTH/") {
		return "tthl/" + strings.TrimPrefix(request, "tthl TTH/")
	}
	if strings.HasPrefix(request, "file TTH/") {
		return "tth/" + strings.TrimPrefix(request, "file TTH/")
	}
	if request == "file files.xml.bz2" {
		return "filelist"
	}
	return "\"" + request + "\""
}

func randomInt(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func numtoa(num interface{}) string {
	return fmt.Sprintf("%d", num)
}

func atoi(s string) int {
	ret, _ := strconv.Atoi(s)
	return ret
}

func atoui(s string) uint {
	ret, _ := strconv.ParseUint(s, 10, 64)
	return uint(ret)
}

func atoui64(s string) uint64 {
	ret, _ := strconv.ParseUint(s, 10, 64)
	return ret
}

func atoi64(s string) int64 {
	ret, _ := strconv.ParseInt(s, 10, 64)
	return ret
}

type connEstablisher struct {
	Wait  chan struct{}
	Conn  net.Conn
	Error error
}

func newConnEstablisher(address string, timeout time.Duration, retries uint) *connEstablisher {
	ce := &connEstablisher{
		Wait: make(chan struct{}),
	}

	go func() {
		ce.Conn, ce.Error = connWithTimeoutAndRetries(address, timeout, retries)
		close(ce.Wait)
	}()

	return ce
}

func connWithTimeoutAndRetries(address string, timeout time.Duration, retries uint) (net.Conn, error) {
	var err error
	for i := uint(0); i < retries; i++ {
		var conn net.Conn
		conn, err = net.DialTimeout("tcp4", address, timeout)
		if err == nil {
			return conn, nil
		}
	}
	return nil, err
}

type bytesWriteCloser struct {
	buf    []byte
	offset int
}

func newBytesWriteCloser(buf []byte) io.WriteCloser {
	return &bytesWriteCloser{buf: buf}
}

func (rc *bytesWriteCloser) Write(in []byte) (int, error) {
	n := copy(rc.buf[rc.offset:], in)
	rc.offset += n
	return n, nil
}

func (rc *bytesWriteCloser) Close() error {
	return nil
}
