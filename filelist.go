package dctoolkit

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

// FileListFile is part of a user file list and represents a shared file.
type FileListFile struct {
	Name string `xml:"Name,attr"`
	Size uint64 `xml:"Size,attr"`
	TTH  string `xml:"TTH,attr"`
}

// FileListDirectory is part of a user file list and represents a shared drectory.
type FileListDirectory struct {
	Name  string               `xml:"Name,attr"`
	Files []*FileListFile      `xml:"File"`
	Dirs  []*FileListDirectory `xml:"Directory"`
}

// FileList is a user file list, containing directories and files.
type FileList struct {
	XMLName   xml.Name             `xml:"FileListing"`
	Version   string               `xml:"Version,attr"`
	CID       string               `xml:"CID,attr"`
	Base      string               `xml:"Base,attr"`
	Generator string               `xml:"Generator,attr"`
	Dirs      []*FileListDirectory `xml:"Directory"`
}

// FileListParse parses a given user file list in XML format into a FileList struct.
func FileListParse(in []byte) (*FileList, error) {
	fl := &FileList{}

	err := xml.Unmarshal(in, fl)
	if err != nil {
		return nil, err
	}
	return fl, err
}

// GetDirectory returns the directory in the file list corresponding to the given path.
func (fl *FileList) GetDirectory(dpath string) (*FileListDirectory, error) {
	components := strings.Split(strings.Trim(dpath, "/"), "/")

	curDir, ok := func() (*FileListDirectory, bool) {
		for _, d := range fl.Dirs {
			if d.Name == components[0] {
				return d, true
			}
		}
		return nil, false
	}()
	if ok == false {
		return nil, fmt.Errorf("directory not found")
	}
	components = components[1:]

	for len(components) > 0 {
		curDir, ok = func() (*FileListDirectory, bool) {
			for _, d := range curDir.Dirs {
				if d.Name == components[0] {
					return d, true
				}
			}
			return nil, false
		}()
		if ok == false {
			return nil, fmt.Errorf("directory not found")
		}
		components = components[1:]
	}
	return curDir, nil
}

// GetFile returns the file in the file list corresponding to the given path.
func (fl *FileList) GetFile(fpath string) (*FileListFile, error) {
	dpath, fname := filepath.Split(fpath)

	dir, err := fl.GetDirectory(dpath)
	if err != nil {
		return nil, err
	}

	for _, f := range dir.Files {
		if f.Name == fname {
			return f, nil
		}
	}
	return nil, fmt.Errorf("file not found")
}

// Export transform the FileList struct into a user file list in the XML format.
func (fl *FileList) Export() ([]byte, error) {
	if fl.Version == "" {
		fl.Version = "1"
	}
	if fl.CID == "" {
		return nil, fmt.Errorf("CID is required")
	}
	if fl.Base == "" {
		fl.Base = ""
	}
	if fl.Generator == "" {
		return nil, fmt.Errorf("Generator is required")
	}

	out, err := xml.MarshalIndent(fl, "", "    ")
	if err != nil {
		return nil, err
	}

	out = append([]byte(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>`+"\n"), out...)
	return out, nil
}
