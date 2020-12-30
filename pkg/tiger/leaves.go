package tiger

import (
	"bufio"
	"bytes"
	"io"
	"os"

	godctiger "github.com/aler9/go-dc/tiger"
)

// Leaves is a sequence of hashes that can be used to validate the single
// parts of a file, and ultimately to compute the file TTH.
type Leaves []Hash

// LeavesLoadFromReader loads TTH leaves from data provided by a Reader.
// please note that this function does NOT compute TTH leaves of the input data,
// it just reads the data and use it as TTH leaves.
func LeavesLoadFromReader(r io.Reader) (Leaves, error) {
	var ret Leaves

	// load hash by hash
	var h Hash
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

// LeavesLoadFromBytes loads TTH leaves from a byte slice.
// please note that this function does NOT compute TTH leaves of the input data,
// it just reads the data and use it as TTH leaves.
func LeavesLoadFromBytes(in []byte) (Leaves, error) {
	return LeavesLoadFromReader(bytes.NewReader(in))
}

// LeavesLoadFromFile loads TTH leaves from a file.
// please note that this function does NOT compute TTH leaves of the input data,
// it just reads the data and use it as TTH leaves.
func LeavesLoadFromFile(fpath string) (Leaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return LeavesLoadFromReader(buf)
}

// LeavesFromReader computes the TTH leaves of data provided by a Reader.
func LeavesFromReader(in io.Reader) (Leaves, error) {
	ttl, err := godctiger.TreeLeaves(in)
	if err != nil {
		return nil, err
	}

	var ret Leaves
	for _, l := range ttl {
		ret = append(ret, Hash(l))
	}
	return ret, nil
}

// LeavesFromBytes computes the TTH leaves of a byte slice.
func LeavesFromBytes(in []byte) (Leaves, error) {
	return LeavesFromReader(bytes.NewReader(in))
}

// LeavesFromFile computes the TTH leaves of a file.
func LeavesFromFile(fpath string) (Leaves, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	return LeavesFromReader(buf)
}

// SaveToWriter saves the TTH leaves into a Writer.
func (l Leaves) SaveToWriter(w io.Writer) error {
	for _, h := range l {
		_, err := w.Write(h[:])
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveToBytes saves the TTH leaves into a byte slice.
func (l Leaves) SaveToBytes() ([]byte, error) {
	var buf bytes.Buffer
	err := l.SaveToWriter(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SaveToFile saves the TTH leaves into a file.
func (l Leaves) SaveToFile(fpath string) error {
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	return l.SaveToWriter(f)
}

// TreeHash converts tiger leaves into a TTH.
func (l Leaves) TreeHash() Hash {
	var ttl godctiger.Leaves
	for _, h := range l {
		ttl = append(ttl, godctiger.Hash(h))
	}

	h := ttl.TreeHash()
	return Hash(h)
}
