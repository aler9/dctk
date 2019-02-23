package dctoolkit

import (
	"bufio"
	"bytes"
	"encoding/base32"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
)

var reTTH = regexp.MustCompile("^" + reStrTTH + "$")

// TTHIsValid checks the validity of a Tiger Tree Hash (TTH), the 39-digits string
// associated to a specific shared file.
func TTHIsValid(in string) bool {
	return reTTH.MatchString(in)
}

// TTHFromBytes computes the Tiger Tree Hash (TTH) of a given byte sequence.
func TTHFromBytes(in []byte) string {
	ret, _ := tthFromReader(bytes.NewReader(in))
	return ret
}

// TTHFromFile computes the Tiger Tree Hash (TTH) of a given file.
func TTHFromFile(fpath string) (string, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return tthFromReader(buf)
}

// TTHLeavesFromBytes computes the TTH leaves of a given byte sequence. The
// leaves are a sequence of concatenated hashes that can be used to validate
// the single parts of a certain file.
func TTHLeavesFromBytes(in []byte) []byte {
	ret, _ := tthLeavesFromReader(bytes.NewReader(in))
	return ret
}

// TTHLeavesFromFile computes the TTH leaves of a given file. The
// leaves are a sequence of concatenated hashes that can be used to validate
// the single parts of a certain file.
func TTHLeavesFromFile(fpath string) ([]byte, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return tthLeavesFromReader(buf)
}

type tthLevel struct {
	b        [24]byte
	occupied bool
}

func tthLeavesFromReader(in io.Reader) ([]byte, error) {
	hasher := tigerNew()
	var out []byte

	firstHash := true
	var buf [1024]byte
	for {
		n, err := in.Read(buf[:])
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n == 0 && firstHash == false { // hash at least one chunk (in case input has zero size)
			break
		}
		firstHash = false

		// level zero hashes are prefixed with 0x00
		hasher.Write([]byte{0x00})
		hasher.Write(buf[:n])
		var sum [24]byte
		hasher.Sum(sum[:0])
		hasher.Reset()

		out = append(out, sum[:]...)
	}
	return out, nil
}

// TTHFromLeaves computes the Tiger Tree Hash (TTH) of a given leaves sequence.
func TTHFromLeaves(leaves []byte) string {
	// ref: https://adc.sourceforge.io/draft-jchapweske-thex-02.html
	hasher := tigerNew()
	var levels []*tthLevel
	levels = append(levels, &tthLevel{}) // add level zero

	for {
		var n int
		if len(leaves) < 24 {
			n = len(leaves)
		} else {
			n = 24
		}
		if n == 0 {
			break
		}

		var sumToAdd [24]byte
		copy(sumToAdd[:], leaves[:n])
		leaves = leaves[n:]

		// upper level level hashes
		for curLevel := 0; curLevel < len(levels); curLevel++ {
			// level has already a hash: compute upper hash and clear level
			if levels[curLevel].occupied == true {
				// upper level hashes are prefixed with 0x01
				hasher.Write([]byte{0x01})
				hasher.Write(levels[curLevel].b[:])
				hasher.Write(sumToAdd[:])
				hasher.Sum(sumToAdd[:0])
				hasher.Reset()

				levels[curLevel].occupied = false

				// add an additional level if necessary
				if len(levels) < (curLevel + 2) {
					levels = append(levels, &tthLevel{})
				}

				// level is free: put here current hash and exit
			} else {
				copy(levels[curLevel].b[:], sumToAdd[:])
				levels[curLevel].occupied = true
				break
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
				hasher.Write([]byte{0x01})
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

func tthFromReader(in io.Reader) (string, error) {
	// ref: https://adc.sourceforge.io/draft-jchapweske-thex-02.html
	hasher := tigerNew()
	var levels []*tthLevel
	levels = append(levels, &tthLevel{}) // add level zero

	// level zero hashes (hashes of chunks of 1024 bytes)
	firstHash := true
	var buf [1024]byte
	for {
		n, err := in.Read(buf[:])
		if err != nil && err != io.EOF {
			return "", err
		}
		if n == 0 && firstHash == false { // hash at least one chunk (in case input has zero size)
			break
		}
		firstHash = false

		// level zero hashes are prefixed with 0x00
		hasher.Write([]byte{0x00})
		hasher.Write(buf[:n])
		var sumToAdd [24]byte
		hasher.Sum(sumToAdd[:0])
		hasher.Reset()

		// upper level level hashes
		for curLevel := 0; curLevel < len(levels); curLevel++ {
			// level has already a hash: compute upper hash and clear level
			if levels[curLevel].occupied == true {
				// upper level hashes are prefixed with 0x01
				hasher.Write([]byte{0x01})
				hasher.Write(levels[curLevel].b[:])
				hasher.Write(sumToAdd[:])
				hasher.Sum(sumToAdd[:0])
				hasher.Reset()

				levels[curLevel].occupied = false

				// add an additional level if necessary
				if len(levels) < (curLevel + 2) {
					levels = append(levels, &tthLevel{})
				}

				// level is free: put here current hash and exit
			} else {
				copy(levels[curLevel].b[:], sumToAdd[:])
				levels[curLevel].occupied = true
				break
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
				hasher.Write([]byte{0x01})
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
	return out, nil
}

// MagnetLink generates a link to a shared file. The link can be shared anywhere
// and can be opened by most of the available DC clients, starting the download.
func MagnetLink(name string, size uint64, tth string) string {
	return fmt.Sprintf("magnet:?xt=urn:tree:tiger:%s&xl=%v&dn=%s",
		tth,
		size,
		url.QueryEscape(name))
}
