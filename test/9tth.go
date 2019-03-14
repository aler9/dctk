// +build ignore

package main

import (
	"fmt"
	"github.com/direct-connect/go-dc/tiger"
	dctk "github.com/gswly/dctoolkit"
	"strings"
)

func main() {
	// test vectors taken from
	// http://adc.sourceforge.net/draft-jchapweske-thex-02.html

	hash := dctk.TTHFromBytes([]byte{})
	if hash != tiger.MustParseBase32("LWPNACQDBZRYXW3VHJVCJ64QBZNGHOHHHZWCLNQ") {
		panic(fmt.Errorf("wrong hash (3): %s", hash))
	}

	hash = dctk.TTHFromBytes([]byte("\x00"))
	if hash != tiger.MustParseBase32("VK54ZIEEVTWNAUI5D5RDFIL37LX2IQNSTAXFKSA") {
		panic(fmt.Errorf("wrong hash (4): %s", hash))
	}

	hash = dctk.TTHFromBytes([]byte(strings.Repeat("A", 1024)))
	if hash != tiger.MustParseBase32("L66Q4YVNAFWVS23X2HJIRA5ZJ7WXR3F26RSASFA") {
		panic(fmt.Errorf("wrong hash (5): %s", hash))
	}

	hash = dctk.TTHFromBytes([]byte(strings.Repeat("A", 1025)))
	if hash != tiger.MustParseBase32("PZMRYHGY6LTBEH63ZWAHDORHSYTLO4LEFUIKHWY") {
		panic(fmt.Errorf("wrong hash (6): %s", hash))
	}

	hash = dctk.TTHFromBytes([]byte(strings.Repeat("A", 10000)))
	if hash != tiger.MustParseBase32("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") {
		panic(fmt.Errorf("wrong hash (7): %s", hash))
	}

	hash = dctk.TTHFromLeaves(dctk.TTHLeavesFromBytes([]byte(strings.Repeat("A", 10000))))
	if hash != tiger.MustParseBase32("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") {
		panic(fmt.Errorf("wrong hash (8): %s", hash))
	}

	fmt.Println("all right")
}
