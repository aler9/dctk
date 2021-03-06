package dctk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk/pkg/tiger"
)

var testCasesTTH = []struct {
	b []byte
	l tiger.Leaves
}{
	{
		[]byte{
			0xc, 0x78, 0x1a, 0xa4, 0x2c, 0x38, 0xcb, 0x51,
			0x15, 0x1c, 0xb0, 0x5f, 0x25, 0xfb, 0x76, 0xec,
			0xed, 0x7b, 0x54, 0x32, 0x17, 0x7f, 0xc0, 0xad,
			0x16, 0x61, 0x4b, 0x1f, 0x68, 0xc5, 0xc2, 0x5e,
			0xaf, 0x61, 0x36, 0x28, 0x6c, 0x9c, 0x12, 0x93,
			0x2f, 0x4f, 0x73, 0xe8, 0x7e, 0x90, 0xa2, 0x73,
			0x7c, 0xf4, 0x15, 0xf7, 0xe7, 0xe2, 0x53, 0xd5,
			0x9e, 0xe1, 0x18, 0x3c, 0x24, 0xf5, 0x44, 0x73,
			0xbb, 0x72, 0x29, 0xa8, 0x2e, 0x10, 0xd2, 0x7d,
			0xde, 0x66, 0x50, 0xdb, 0x98, 0xf7, 0x5c, 0x63,
			0x36, 0xe2, 0x67, 0xff, 0xe1, 0xe1, 0xbd, 0xb2,
			0x7d, 0x2c, 0xae, 0xd1, 0xfd, 0x98, 0x2e, 0x34,
		},
		tiger.Leaves{
			tiger.HashMust("BR4BVJBMHDFVCFI4WBPSL63W5TWXWVBSC574BLI"),
			tiger.HashMust("CZQUWH3IYXBF5L3BGYUGZHASSMXU647IP2IKE4Y"),
			tiger.HashMust("PT2BL57H4JJ5LHXBDA6CJ5KEOO5XEKNIFYINE7I"),
			tiger.HashMust("3ZTFBW4Y65OGGNXCM776DYN5WJ6SZLWR7WMC4NA"),
		},
	},
}

func TestTigerLeavesLoad(t *testing.T) {
	for _, c := range testCasesTTH {
		l, err := tiger.LeavesLoadFromBytes(c.b)
		require.NoError(t, err)
		require.Equal(t, c.l, l)
	}
}

func TestTigerLeavesSave(t *testing.T) {
	for _, c := range testCasesTTH {
		b, err := c.l.SaveToBytes()
		require.NoError(t, err)
		require.Equal(t, c.b, b)
	}
}
