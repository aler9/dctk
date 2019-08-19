// +build ignore

package main

import (
	"fmt"
	"os"

	dctk "github.com/gswly/dctoolkit"
)

func main() {
	filepath := "/share/test file.txt"

	// get file size
	finfo, err := os.Stat(filepath)
	if err != nil {
		panic(err)
	}

	// compute and print file TTH
	tth, err := dctk.TTHFromFile(filepath)
	if err != nil {
		panic(err)
	}
	fmt.Println("tth:", tth)

	// get and print the magnet URL corresponding to the given file
	magnetLink := dctk.MagnetLink("filename", uint64(finfo.Size()), tth)
	fmt.Println("magnet link:", magnetLink)
}
