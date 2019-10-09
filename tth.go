package dctoolkit

import (
	"bufio"
	"bytes"
	"fmt"
	"hash"
	"net/url"
	"os"

	"github.com/aler9/go-dc/tiger"
)

// tiger hash used throughout the library.
func newTiger() hash.Hash {
	return tiger.New()
}

// TigerLeaves is a sequence of hashes that can be used to validate the single
// parts of a file, and ultimately to compute the file TTH.
type TigerLeaves tiger.Leaves

// TTHLeavesFromBytes computes the TTH leaves of a given byte sequence.
func TTHLeavesFromBytes(in []byte) TigerLeaves {
	ret, _ := tiger.TreeLeaves(bytes.NewReader(in))
	return TigerLeaves(ret)
}

// TTHLeavesFromFile computes the TTH leaves of a given file.
func TTHLeavesFromFile(fpath string) (TigerLeaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	ret, err := tiger.TreeLeaves(buf)
	return TigerLeaves(ret), err
}

// TreeHash converts tiger leaves into a TTH
func (l TigerLeaves) TreeHash() TigerHash {
	h := tiger.Leaves(l).TreeHash()
	return TigerHash(h)
}

// TigerHash is the result of the hash cryptographic function.
// In particular, it is used to save a Tiger Tree Hash (TTH), the univoque
// identifier associated to a specific file content.
type TigerHash tiger.Hash

// TTHFromBase32 imports a TigerHash in base32 encoding.
func TigerHashFromBase32(in string) (TigerHash, error) {
	ret := new(tiger.Hash)
	err := ret.FromBase32(in)
	return TigerHash(*ret), err
}

// TigerHashMust is like TTHFromBase32 but panics in case of error.
func TigerHashMust(in string) TigerHash {
	ret, err := TigerHashFromBase32(in)
	if err != nil {
		panic(err)
	}
	return ret
}

// String converts a hash to its base32 representation.
func (h TigerHash) String() string {
	return tiger.Hash(h).String()
}

// MarshalText implements encoding.TextMarshaler.
func (h TigerHash) MarshalText() ([]byte, error) {
	return tiger.Hash(h).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (h *TigerHash) UnmarshalText(text []byte) error {
	return (*tiger.Hash)(h).UnmarshalText(text)
}

// TTHFromBytes computes the Tiger Tree Hash (TTH) of a given byte sequence.
func TTHFromBytes(in []byte) TigerHash {
	ret, _ := tiger.TreeHash(bytes.NewReader(in))
	return TigerHash(ret)
}

// TTHFromFile computes the Tiger Tree Hash (TTH) of a given file.
func TTHFromFile(fpath string) (TigerHash, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return TigerHash{}, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	ret, err := tiger.TreeHash(buf)
	return TigerHash(ret), err
}

// MagnetLink generates a link to a shared file. The link can be shared anywhere
// and can be opened by most of the available DC clients, starting the download.
func MagnetLink(name string, size uint64, tth TigerHash) string {
	return fmt.Sprintf("magnet:?xt=urn:tree:tiger:%s&xl=%d&dn=%s",
		tth,
		size,
		url.QueryEscape(name))
}
