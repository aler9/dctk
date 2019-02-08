package dctoolkit

import (
    "os"
    "io"
    "fmt"
    "bytes"
    "bufio"
    "regexp"
    "net/url"
    "encoding/base32"
    "github.com/cxmcc/tiger"
)

var reTTH = regexp.MustCompile("^"+reStrTTH+"$")

func MagnetLink(name string, size uint64, tth string) string {
    return fmt.Sprintf("magnet:?xt=urn:tree:tiger:%s&xl=%v&dn=%s",
        tth,
        size,
        url.QueryEscape(name))
}

func TTHIsValid(in string) bool {
    return reTTH.MatchString(in)
}

func TTHFromBytes(in []byte) string {
    ret,_ := tthFromReader(bytes.NewReader(in))
    return ret
}

func TTHFromFile(fpath string) (string,error) {
    f,err := os.Open(fpath)
    if err != nil {
        return "",err
    }
    defer f.Close()

    // buffer to optimize disk read
    buf := bufio.NewReaderSize(f, 1024 * 1024)

    return tthFromReader(buf)
}

func TTHLeavesFromBytes(in []byte) []byte {
    ret,_ := tthLeavesFromReader(bytes.NewReader(in))
    return ret
}

func TTHLeavesFromFile(fpath string) ([]byte,error) {
    f,err := os.Open(fpath)
    if err != nil {
        return nil,err
    }
    defer f.Close()

    // buffer to optimize disk read
    buf := bufio.NewReaderSize(f, 1024 * 1024)

    return tthLeavesFromReader(buf)
}

type tthLevel struct {
    b [24]byte
    occupied bool
}

func tthLeavesFromReader(in io.Reader) ([]byte,error) {
    hasher := tiger.New()
    var out []byte

    firstHash := true
    var buf [1024]byte
    for {
        n,err := in.Read(buf[:])
        if err != nil && err != io.EOF {
            return nil,err
        }
        if n == 0 && firstHash == false { // hash at least one chunk (in case input has zero size)
            break
        }
        firstHash = false

        // level zero hashes are prefixed with 0x00
        hasher.Write([]byte{ 0x00 })
        hasher.Write(buf[:n])
        var sum [24]byte
        hasher.Sum(sum[:0])
        hasher.Reset()

        out = append(out, sum[:]...)
    }
    return out, nil
}

// ref: https://adc.sourceforge.io/draft-jchapweske-thex-02.html
func TTHFromLeaves(leafs []byte) string {
    hasher := tiger.New()
    var levels []*tthLevel
    levels = append(levels, &tthLevel{}) // add level zero

    for {
        var n int
        if len(leafs) < 24 {
            n = len(leafs)
        } else {
            n = 24
        }
        if n == 0 {
            break
        }

        var sumToAdd [24]byte
        copy(sumToAdd[:], leafs[:n])
        leafs = leafs[n:]

        // upper level level hashes
        for curLevel := 0; curLevel < len(levels); curLevel++ {
            // level is free: put here current hash and exit
            if levels[curLevel].occupied == false {
                copy(levels[curLevel].b[:], sumToAdd[:])
                levels[curLevel].occupied = true
                break

            // level has already a hash: compute upper hash and clear level
            } else {
                // upper level hashes are prefixed with 0x01
                hasher.Write([]byte{ 0x01 })
                hasher.Write(levels[curLevel].b[:])
                hasher.Write(sumToAdd[:])
                hasher.Sum(sumToAdd[:0])
                hasher.Reset()

                levels[curLevel].occupied = false

                // add an additional level if necessary
                if len(levels) < (curLevel+2) {
                    levels = append(levels, &tthLevel{})
                }
            }
        }
    }

    // compute or move up remaining hashes, up to topLevel
    topLevel := &tthLevel{}
    for curLevel := 0; curLevel < len(levels); curLevel++ {
        if levels[curLevel].occupied == true {
            // compute
            if topLevel.occupied == true {
                // upper level hashes are prefixed with 0x01
                hasher.Write([]byte{ 0x01 })
                hasher.Write(levels[curLevel].b[:])
                hasher.Write(topLevel.b[:])
                hasher.Sum(topLevel.b[:0])
                hasher.Reset()

            // move up
            } else {
                copy(topLevel.b[:], levels[curLevel].b[:])
                topLevel.occupied = true
            }
        }
    }

    out := base32.StdEncoding.EncodeToString(topLevel.b[:])[:39]
    return out
}

// ref: https://adc.sourceforge.io/draft-jchapweske-thex-02.html
func tthFromReader(in io.Reader) (string,error) {
    hasher := tiger.New()
    var levels []*tthLevel
    levels = append(levels, &tthLevel{}) // add level zero

    // level zero hashes (hashes of chunks of 1024 bytes)
    firstHash := true
    var buf [1024]byte
    for {
        n,err := in.Read(buf[:])
        if err != nil && err != io.EOF {
            return "",err
        }
        if n == 0 && firstHash == false { // hash at least one chunk (in case input has zero size)
            break
        }
        firstHash = false

        // level zero hashes are prefixed with 0x00
        hasher.Write([]byte{ 0x00 })
        hasher.Write(buf[:n])
        var sumToAdd [24]byte
        hasher.Sum(sumToAdd[:0])
        hasher.Reset()

        // upper level level hashes
        for curLevel := 0; curLevel < len(levels); curLevel++ {
            // level is free: put here current hash and exit
            if levels[curLevel].occupied == false {
                copy(levels[curLevel].b[:], sumToAdd[:])
                levels[curLevel].occupied = true
                break

            // level has already a hash: compute upper hash and clear level
            } else {
                // upper level hashes are prefixed with 0x01
                hasher.Write([]byte{ 0x01 })
                hasher.Write(levels[curLevel].b[:])
                hasher.Write(sumToAdd[:])
                hasher.Sum(sumToAdd[:0])
                hasher.Reset()

                levels[curLevel].occupied = false

                // add an additional level if necessary
                if len(levels) < (curLevel+2) {
                    levels = append(levels, &tthLevel{})
                }
            }
        }
    }

    // compute or move up remaining hashes, up to topLevel
    topLevel := &tthLevel{}
    for curLevel := 0; curLevel < len(levels); curLevel++ {
        if levels[curLevel].occupied == true {
            // compute
            if topLevel.occupied == true {
                // upper level hashes are prefixed with 0x01
                hasher.Write([]byte{ 0x01 })
                hasher.Write(levels[curLevel].b[:])
                hasher.Write(topLevel.b[:])
                hasher.Sum(topLevel.b[:0])
                hasher.Reset()

            // move up
            } else {
                copy(topLevel.b[:], levels[curLevel].b[:])
                topLevel.occupied = true
            }
        }
    }

    out := base32.StdEncoding.EncodeToString(topLevel.b[:])[:39]
    return out,nil
}
