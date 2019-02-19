package main

import (
    "fmt"
    "gopkg.in/alecthomas/kingpin.v2"
    dctk "github.com/gswly/dctoolkit"
)

var (
    fpath = kingpin.Arg("fpath", "Path to a file").Required().String()
)

func main() {
    kingpin.CommandLine.Help = "Computes the Tiger Tree Hash (TTH) of a given file."
    kingpin.Parse()

    tth,err := dctk.TTHFromFile(*fpath)
    if err != nil {
        panic(err)
    }
    fmt.Println(tth)
}
