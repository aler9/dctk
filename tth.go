package dctoolkit

import (
	"bufio"
	"bytes"
	"encoding/base32"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
)

var reTTH = regexp.MustCompile("^" + reStrTTH + "$")

type tthLevel struct {
	b        [24]byte
	occupied bool
}

// TTHLeaves is a sequence of concatenated hashes that can be used to validate
// the single parts of a certain file.
type TTHLeaves []byte

// TTHLeavesFromBytes computes the TTH leaves of a given byte sequence.
func TTHLeavesFromBytes(in []byte) TTHLeaves {
	ret, _ := TTHLeavesFromReader(bytes.NewReader(in))
	return ret
}

// TTHLeavesFromFile computes the TTH leaves of a given file.
func TTHLeavesFromFile(fpath string) (TTHLeaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return TTHLeavesFromReader(buf)
}

// TTHLeavesFromReader computes the TTH leaves of data provided by an io.Reader.
func TTHLeavesFromReader(in io.Reader) (TTHLeaves, error) {
	hasher := newTiger()
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
	return TTHLeaves(out), nil
}

// TTH is a Tiger Tree Hash (TTH), the 39-digits string
// associated to a specific shared file.
type TTH string

// TTHImport imports a Tiger Tree Hash (TTH) in string format.
func TTHImport(in string) (TTH, error) {
	if reTTH.MatchString(in) == false {
		return "", fmt.Errorf("invalid TTH")
	}
	return TTH(in), nil
}

// TTHFromBytes computes the Tiger Tree Hash (TTH) of a given byte sequence.
func TTHFromBytes(in []byte) TTH {
	ret, _ := TTHFromReader(bytes.NewReader(in))
	return ret
}

// TTHFromFile computes the Tiger Tree Hash (TTH) of a given file.
func TTHFromFile(fpath string) (TTH, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return TTHFromReader(buf)
}

// TTHFromLeaves computes the Tiger Tree Hash (TTH) of a given leaves sequence.
func TTHFromLeaves(leaves TTHLeaves) TTH {
	// ref: https://adc.sourceforge.io/draft-jchapweske-thex-02.html
	hasher := newTiger()
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

		// upper level hashes
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
	return TTH(out)
}

// TTHFromReader computes the Tiger Tree Hash (TTH) of data provided by an io.Reader.
func TTHFromReader(in io.Reader) (TTH, error) {
	// ref: https://adc.sourceforge.io/draft-jchapweske-thex-02.html
	hasher := newTiger()
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
	return TTH(out), nil
}

func (t TTH) String() string {
	return string(t)
}

// UnmarshalXMLAttr implements the xml.UnmarshalerAttr interface.
func (t *TTH) UnmarshalXMLAttr(attr xml.Attr) error {
	tth,err := TTHImport(attr.Value)
	if err != nil {
		return err
	}
	*t = tth
	return nil
}

// MarshalXMLAttr implements the xml.MarshalerAttr interface.
func (t TTH) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{name, string(t)}, nil
}

// MagnetLink generates a link to a shared file. The link can be shared anywhere
// and can be opened by most of the available DC clients, starting the download.
func MagnetLink(name string, size uint64, tth TTH) string {
	return fmt.Sprintf("magnet:?xt=urn:tree:tiger:%s&xl=%v&dn=%s",
		tth,
		size,
		url.QueryEscape(name))
}
