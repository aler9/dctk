package main

import (
	"fmt"

	dctk "github.com/gswly/dctoolkit"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	fpath = kingpin.Arg("fpath", "Path to a file").Required().String()
)

func main() {
	kingpin.CommandLine.Help = "Compute the Tiger Tree Hash (TTH) of a given file."
	kingpin.Parse()

	tth, err := dctk.TTHFromFile(*fpath)
	if err != nil {
		panic(err)
	}
	fmt.Println(tth)
}
