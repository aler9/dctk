package tiger

import (
	"bufio"
	"bytes"
	"fmt"
	"hash"
	"net/url"
	"os"

	godctiger "github.com/aler9/go-dc/tiger"
)

// NewHash allocates a tiger hash instance.
func NewHash() hash.Hash {
	return godctiger.New()
}

// Hash is the result of the hash cryptographic function.
// In particular, it is used to save a Tiger Tree Hash (TTH), the univoque
// identifier associated to a specific file content.
type Hash godctiger.Hash

// HashFromBase32 imports a Hash in base32 encoding.
func HashFromBase32(in string) (Hash, error) {
	ret := new(godctiger.Hash)
	err := ret.FromBase32(in)
	return Hash(*ret), err
}

// HashMust is like TTHFromBase32 but panics in case of error.
func HashMust(in string) Hash {
	ret, err := HashFromBase32(in)
	if err != nil {
		panic(err)
	}
	return ret
}

// String converts a hash to its base32 representation.
func (h Hash) String() string {
	return godctiger.Hash(h).String()
}

// MarshalText implements encoding.TextMarshaler.
func (h Hash) MarshalText() ([]byte, error) {
	return godctiger.Hash(h).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (h *Hash) UnmarshalText(text []byte) error {
	return (*godctiger.Hash)(h).UnmarshalText(text)
}

// HashFromBytes computes the Tiger Tree Hash (TTH) of a byte slice.
func HashFromBytes(in []byte) Hash {
	ret, _ := godctiger.TreeHash(bytes.NewReader(in))
	return Hash(ret)
}

// tiger.HashFromFile computes the Tiger Tree Hash (TTH) of a file.
func HashFromFile(fpath string) (Hash, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return Hash{}, err
	}
	defer f.Close()

	// buffer to optimize disk read
	buf := bufio.NewReaderSize(f, 1024*1024)

	ret, err := godctiger.TreeHash(buf)
	return Hash(ret), err
}

// MagnetLink generates a link to a shared file. The link can be shared anywhere
// and can be opened by most of the available DC clients, starting the download.
func MagnetLink(name string, size uint64, tth Hash) string {
	return fmt.Sprintf("magnet:?xt=urn:tree:tiger:%s&xl=%d&dn=%s",
		tth,
		size,
		url.QueryEscape(name))
}
