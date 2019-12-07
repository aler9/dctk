package dctoolkit

import (
	"bufio"
	"bytes"
	"fmt"
	"hash"
	"io"
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
type TigerLeaves []TigerHash

// TTHLeavesLoadFromReader loads TTH leaves from data provided by a Reader.
// please note that this function does NOT compute TTH leaves of the input data,
// it just reads the data and use it as TTH leaves.
func TTHLeavesLoadFromReader(r io.Reader) (TigerLeaves, error) {
	var ret TigerLeaves

	// load hash by hash
	var h TigerHash
	for {
		_, err := io.ReadFull(r, h[:])
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		ret = append(ret, h)
	}

	return ret, nil
}

// TTHLeavesLoadFromBytes loads TTH leaves from a byte slice.
// please note that this function does NOT compute TTH leaves of the input data,
// it just reads the data and use it as TTH leaves.
func TTHLeavesLoadFromBytes(in []byte) (TigerLeaves, error) {
	return TTHLeavesLoadFromReader(bytes.NewReader(in))
}

// TTHLeavesLoadFromFile loads TTH leaves from a file.
// please note that this function does NOT compute TTH leaves of the input data,
// it just reads the data and use it as TTH leaves.
func TTHLeavesLoadFromFile(fpath string) (TigerLeaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return TTHLeavesLoadFromReader(buf)
}

// TTHLeavesFromReader computes the TTH leaves of data provided by a Reader.
func TTHLeavesFromReader(in io.Reader) (TigerLeaves, error) {
	ttl, err := tiger.TreeLeaves(in)
	if err != nil {
		return nil, err
	}

	var ret TigerLeaves
	for _, l := range ttl {
		ret = append(ret, TigerHash(l))
	}
	return ret, nil
}

// TTHLeavesFromBytes computes the TTH leaves of a byte slice.
func TTHLeavesFromBytes(in []byte) (TigerLeaves, error) {
	return TTHLeavesFromReader(bytes.NewReader(in))
}

// TTHLeavesFromFile computes the TTH leaves of a file.
func TTHLeavesFromFile(fpath string) (TigerLeaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return TTHLeavesFromReader(buf)
}

// SaveToWriter saves the TTH leaves into a Writer.
func (l TigerLeaves) SaveToWriter(w io.Writer) error {
	for _, h := range l {
		_, err := w.Write(h[:])
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveToBytes saves the TTH leaves into a byte slice.
func (l TigerLeaves) SaveToBytes() ([]byte, error) {
	var buf bytes.Buffer
	err := l.SaveToWriter(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SaveToFile saves the TTH leaves into a file.
func (l TigerLeaves) SaveToFile(fpath string) error {
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	return l.SaveToWriter(f)
}

// TreeHash converts tiger leaves into a TTH
func (l TigerLeaves) TreeHash() TigerHash {
	var ttl tiger.Leaves
	for _, h := range l {
		ttl = append(ttl, tiger.Hash(h))
	}

	h := tiger.Leaves(ttl).TreeHash()
	return TigerHash(h)
}

// TigerHash is the result of the hash cryptographic function.
// In particular, it is used to save a Tiger Tree Hash (TTH), the univoque
// identifier associated to a specific file content.
type TigerHash tiger.Hash

// TigerHashFromBase32 imports a TigerHash in base32 encoding.
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

// TTHFromBytes computes the Tiger Tree Hash (TTH) of a byte slice.
func TTHFromBytes(in []byte) TigerHash {
	ret, _ := tiger.TreeHash(bytes.NewReader(in))
	return TigerHash(ret)
}

// TTHFromFile computes the Tiger Tree Hash (TTH) of a file.
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
