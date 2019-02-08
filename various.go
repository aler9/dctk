package dctoolkit

import (
    "fmt"
    "strings"
    "time"
    "net"
    "strconv"
    "math/rand"
    "encoding/base32"
)

const reStrNick = "[^\\$ \\|]+"
const reStrIp = "[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}"
const reStrPort = "[0-9]{1,5}"
const reStrTTH = "[A-Z0-9]{39}"

type errorTerminated struct {}

func (err *errorTerminated) Error() string {
    return "terminated"
}

func dcEscape(in string) string {
    in = strings.Replace(in, "&", "&amp;", -1)
    in = strings.Replace(in, "$", "&#36;", -1)
    in = strings.Replace(in, "|", "&#124;", -1)
    return in
}

// http://nmdc.sourceforge.net/Versions/NMDC-1.3.html#_key
// https://web.archive.org/web/20150529002427/http://wiki.gusari.org/index.php?title=LockToKey%28%29
func dcComputeKey(lock []byte) string {
    // the key has exactly as many characters as the lock
    key := make([]byte, len(lock))

    // Except for the first, each key character is computed from the corresponding lock character and the one before it
    key[0] = 0
    for n := 1; n < len(key); n++ {
        key[n] = lock[n] ^ lock[n-1]
    }

    // The first key character is calculated from the first lock character and the last two lock characters
    key[0] = lock[0] ^ lock[len(lock)-1] ^ lock[len(lock)-2] ^ 5

    // Next, every character in the key must be nibble-swapped
    for n := 0; n < len(key); n++ {
        key[n] = ((key[n] << 4) & 240) | ((key[n] >> 4) & 15)
    }

    // the characters with the decimal ASCII values of 0, 5, 36, 96, 124, and 126
    // cannot be sent to the server. Each character with this value must be
    // substituted with the string /%DCN000%/, /%DCN005%/, /%DCN036%/, /%DCN096%/, /%DCN124%/, or /%DCN126%/
    var res []byte
    for _,byt := range key {
        if byt == 0 || byt == 5 || byt == 36 || byt == 96 || byt == 124 || byt == 126 {
            res = append(res, []byte(fmt.Sprintf("/%%DCN%.3d%%/", byt))...)
        } else {
            res = append(res, byt)
        }
    }
    return string(res)
}

func dcRandomClientId() string {
    var randomBytes [24]byte
    rand.Read(randomBytes[:])
    return base32.StdEncoding.EncodeToString(randomBytes[:])[:39]
}

func dcReadableRequest(request string) string {
    if strings.HasPrefix(request, "tthl TTH/") {
        return "tthl/" + strings.TrimPrefix(request, "tthl TTH/")
    }
    if strings.HasPrefix(request, "file TTH/") {
        return "tth/" + strings.TrimPrefix(request, "file TTH/")
    }
    return "filelist"
}

func randomInt(min, max int) int {
    rand.Seed(time.Now().Unix())
    return rand.Intn(max - min) + min
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
