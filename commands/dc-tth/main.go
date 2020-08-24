package main

import (
	"fmt"

	"gopkg.in/alecthomas/kingpin.v2"

	dctk "github.com/aler9/dctoolkit"
	"github.com/aler9/dctoolkit/tiger"
)

var (
	fpath = kingpin.Arg("fpath", "Path to a file").Required().String()
)

func main() {
	kingpin.CommandLine.Help = "Compute the Tiger Tree Hash (TTH) of a given file."
	kingpin.Parse()

	tth, err := tiger.HashFromFile(*fpath)
	if err != nil {
		panic(err)
	}
	fmt.Println(tth)
}
