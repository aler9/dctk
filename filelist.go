package dctk

import (
    "encoding/xml"
    "path/filepath"
)

type flFile struct {
    Name    string      `xml:"Name,attr"`
    Size    uint64      `xml:"Size,attr"`
    TTH     string      `xml:"TTH,attr"`
}

type flDirectory struct {
    Name    string           `xml:"Name,attr"`
    Files   []*flFile        `xml:"File"`
    Dirs    []*flDirectory   `xml:"Directory"`
}

type FileListing struct {
    Version     string          `xml:"Version,attr"`
    CID         string          `xml:"CID,attr"`
    Base        string          `xml:"Base,attr"`
    Generator   string          `xml:"Generator,attr"`
    Dirs        []*flDirectory  `xml:"Directory"`
}

func filelistGenerate(clientId string, generator string, tree map[string]*shareRootDirectory) ([]byte,error) {
    fl := &FileListing{
        Version: "1",
        CID: clientId,
        Base: "/",
        Generator: generator,
    }

    var scanDir func(path string, dir *shareDirectory) *flDirectory
    scanDir = func(path string, dir *shareDirectory) *flDirectory {
        fd := &flDirectory{}
        for name,sdir := range dir.dirs {
            sfd := scanDir(filepath.Join(path, name), sdir)
            sfd.Name = name
            fd.Dirs = append(fd.Dirs, sfd)
        }
        for name,file := range dir.files {
            fd.Files = append(fd.Files, &flFile{
                Name: name,
                Size: file.size,
                TTH: file.tth,
            })
        }
        return fd
    }

    for alias,dir := range tree {
        fld := scanDir(dir.path, dir.shareDirectory)
        fld.Name = alias
        fl.Dirs = append(fl.Dirs, fld)
    }

    out,err := xml.MarshalIndent(fl, "", "    ")
	if err != nil {
        return nil,err
	}

    out = append([]byte(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>` + "\n"), out...)
    return out,nil
}
