// +build ignore

package main

import (
	"fmt"
	dctk "github.com/gswly/dctoolkit"
	"reflect"
)

func main() {
	inout := []byte(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<FileListing Version="1" CID="testcid" Base="/" Generator="testgen">
    <Directory Name="share">
        <File Name="file 1" Size="30" TTH="UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"></File>
        <File Name="file 2" Size="30" TTH="UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"></File>
    </Directory>
</FileListing>`)

	fl, err := dctk.FileListParse(inout)
	if err != nil {
		panic(err)
	}

	cmp, err := fl.Export()
	if err != nil {
		panic(cmp)
	}

	if reflect.DeepEqual(cmp, inout) == false {
		panic(fmt.Errorf("input and output are different"))
	}
}
