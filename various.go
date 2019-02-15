package dctoolkit

import (
    "fmt"
    "strings"
    "time"
    "net"
    "hash"
    "strconv"
    "math/rand"
    "encoding/base32"
    "github.com/cxmcc/tiger"
)

const reStrNick = "[^\\$ \\|\n]+"
const reStrIp = "[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}"
const reStrPort = "[0-9]{1,5}"
const reStrTTH = "[A-Z0-9]{39}"

const dirTTH = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var errorTerminated = fmt.Errorf("terminated")
var errorArgsFormat = fmt.Errorf("not formatted correctly")

// base32 without padding, which can be one or multiple =
func dcBase32Encode(in []byte) string {
    return strings.TrimSuffix(base32.StdEncoding.EncodeToString(in), "=")
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

func connRemoteAddr(conn net.Conn) string {
    return conn.RemoteAddr().(*net.TCPAddr).String()
}
