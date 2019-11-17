package dctoolkit_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestFileList(t *testing.T) {
	inout := []byte(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<FileListing Version="1" CID="testcid" Base="/" Generator="testgen">
    <Directory Name="share">
        <File Name="file 1" Size="30" TTH="UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"></File>
        <File Name="file 2" Size="30" TTH="UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"></File>
    </Directory>
</FileListing>`)

	fl, err := dctk.FileListParse(inout)
	require.NoError(t, err)

	cmp, err := fl.Export()
	require.NoError(t, err)

	require.True(t, reflect.DeepEqual(cmp, inout))
}
