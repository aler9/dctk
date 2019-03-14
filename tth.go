package dctoolkit

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/direct-connect/go-dc/tiger"
	"hash"
	"net/url"
	"os"
)

// tiger hash used throughout the library
func newTiger() hash.Hash {
	return tiger.New()
}

// TTHLeavesFromBytes computes the TTH leaves of a given byte sequence.
func TTHLeavesFromBytes(in []byte) tiger.Leaves {
	ret, _ := tiger.TreeLeaves(bytes.NewReader(in))
	return ret
}

// TTHLeavesFromFile computes the TTH leaves of a given file.
func TTHLeavesFromFile(fpath string) (tiger.Leaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return tiger.TreeLeaves(buf)
}

// TTHFromBytes computes the Tiger Tree Hash (TTH) of a given byte sequence.
func TTHFromBytes(in []byte) tiger.Hash {
	ret, _ := tiger.TreeHash(bytes.NewReader(in))
	return ret
}

// TTHFromFile computes the Tiger Tree Hash (TTH) of a given file.
func TTHFromFile(fpath string) (tiger.Hash, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return tiger.Hash{}, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return tiger.TreeHash(buf)
}

// TODO: move into go-dc
func TTHFromLeaves(in tiger.Leaves) tiger.Hash {
	// deep copy leaves since the slice is modified in order to compute the hash
	lvl := append(in[:0:0], in...)
	buf := make([]byte, 2*tiger.Size+1)

	for len(lvl) > 1 {
		for i := 0; i < len(lvl); i += 2 {
			if i+1 >= len(lvl) {
				lvl[i/2] = lvl[i]
			} else {
				buf[0] = 0x01
				copy(buf[1:], lvl[i][:])
				copy(buf[1+tiger.Size:], lvl[i+1][:])
				lvl[i/2] = tiger.HashBytes(buf)
			}
		}
		n := len(lvl) / 2
		if len(lvl)%2 != 0 {
			n++
		}
		lvl = lvl[:n]
	}
	return lvl[0]
}

// MagnetLink generates a link to a shared file. The link can be shared anywhere
// and can be opened by most of the available DC clients, starting the download.
func MagnetLink(name string, size uint64, tth tiger.Hash) string {
	return fmt.Sprintf("magnet:?xt=urn:tree:tiger:%s&xl=%v&dn=%s",
		tth,
		size,
		url.QueryEscape(name))
}
