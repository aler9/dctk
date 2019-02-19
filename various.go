package dctoolkit

import (
    "fmt"
    "strings"
    "time"
    "net"
    "hash"
    "strconv"
    "regexp"
    "io"
    "math/rand"
    "encoding/base32"
    "github.com/cxmcc/tiger"
)

const reStrNick = "[^\\$ \\|\n]+"
const reStrAddress = "[a-z0-9\\.-_]+"
const reStrIp = "[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}"
const reStrPort = "[0-9]{1,5}"
const reStrTTH = "[A-Z0-9]{39}"

var reSharedCmdAdcGet = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) (-1|[0-9]+)( ZL1)?$")
var reSharedCmdAdcSnd = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) ([0-9]+)( ZL1)?$")

const dirTTH = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var errorTerminated = fmt.Errorf("terminated")
var errorArgsFormat = fmt.Errorf("not formatted correctly")

// base32 without padding, which can be one or multiple =
func dcBase32Encode(in []byte) string {
    return strings.TrimRight(base32.StdEncoding.EncodeToString(in), "=")
}

// base32 without padding, which can be one or multiple =
func dcBase32Decode(in string) []byte {
    // add missing padding
    if len(in) % 8 != 0 {
        mlen := (8 - (len(in) % 8))
        for n := 0; n < mlen; n++ {
            in += "="
        }
    }
    out,_ := base32.StdEncoding.DecodeString(in)
    return out
}

func dcReadableQuery(request string) string {
    if strings.HasPrefix(request, "tthl TTH/") {
        return "tthl/" + strings.TrimPrefix(request, "tthl TTH/")
    }
    if strings.HasPrefix(request, "file TTH/") {
        return "tth/" + strings.TrimPrefix(request, "file TTH/")
    }
    return "filelist"
}

// tiger hash used through the library
func tigerNew() hash.Hash {
    return tiger.New()
}

func randomInt(min, max int) int {
    rand.Seed(time.Now().Unix())
    return rand.Intn(max - min) + min
}

func numtoa(num interface{}) string {
    return fmt.Sprintf("%d", num)
}

func atoi(s string) int {
    ret,_ := strconv.Atoi(s)
    return ret
}

func atoui(s string) uint {
    ret,_ := strconv.ParseUint(s, 10, 64)
    return uint(ret)
}

func atoui64(s string) uint64 {
    ret,_ := strconv.ParseUint(s, 10, 64)
    return ret
}

func atoi64(s string) int64 {
    ret,_ := strconv.ParseInt(s, 10, 64)
    return ret
}

type connEstablisher struct {
    Wait    chan struct{}
    Conn    net.Conn
    Error   error
}

func newConnEstablisher(address string, timeout time.Duration, retries uint) *connEstablisher {
    ce := &connEstablisher{
        Wait: make(chan struct{}, 1),
    }

    go func() {
        ce.Conn,ce.Error = connWithTimeoutAndRetries(address, timeout, retries)
        ce.Wait <- struct{}{}
    }()
    return ce
}

func connWithTimeoutAndRetries(address string, timeout time.Duration, retries uint) (net.Conn, error) {
    var err error
    for i := uint(0); i < retries; i++ {
        var conn net.Conn
        conn,err = net.DialTimeout("tcp4", address, timeout)
        if err == nil {
            return conn, nil
        }
    }
    return nil, err
}

type bytesReadCloser struct {
    buf     []byte
    offset  int
}

func newBytesWriteCloser(buf []byte) io.WriteCloser {
    return &bytesReadCloser{ buf: buf }
}

func (rc *bytesReadCloser) Write(in []byte) (int,error) {
    n := copy(rc.buf[rc.offset:], in)
    rc.offset += n
    return n, nil
}

func (rc *bytesReadCloser) Close() error {
    return nil
}
