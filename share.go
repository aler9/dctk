package dctoolkit

import (
	"bytes"
	"github.com/dsnet/compress/bzip2"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type shareFile struct {
	size      uint64
	modTime   time.Time
	tth       TTH
	tthl      TTHLeaves
	realPath  string
	aliasPath string
}

type shareDirectory struct {
	dirs      map[string]*shareDirectory
	files     map[string]*shareFile
	aliasPath string
	size      uint64
}

type shareIndexer struct {
	client             *Client
	terminateRequested bool
	terminateChan      chan struct{}
	indexChan          chan struct{}
	indexRequested     bool
}

func newshareIndexer(client *Client) error {
	client.shareIndexer = &shareIndexer{
		client: client,
		// must be buffered since it could otherwise cause a deadlock:
		// - after <-indexChan and before Safe()
		terminateChan: make(chan struct{}, 1),
		indexChan:     make(chan struct{}),
	}
	client.shareIndexer.index()
	return nil
}

func (sm *shareIndexer) terminate() {
	if sm.terminateRequested == true {
		return
	}
	sm.terminateRequested = true
	sm.terminateChan <- struct{}{}
}

func (sm *shareIndexer) do() {
	defer sm.client.wg.Done()

	for {
		select {
		case <-sm.terminateChan:
			return
		case <-sm.indexChan:
		}

		sm.index()
	}
}

func (sm *shareIndexer) index() {
	copyRoots := make(map[string]string)
	sm.client.Safe(func() {
		sm.indexRequested = false

		// create a copy of shareRoots
		for k, v := range sm.client.shareRoots {
			copyRoots[k] = v
		}
	})

	// generate new tree
	shareTree, shareCount, shareSize := func() (map[string]*shareDirectory, uint, uint64) {
		tree := make(map[string]*shareDirectory)
		count := uint(0)
		size := uint64(0)
		var scanDir func(apath string, dpath string, oldDir *shareDirectory) (*shareDirectory, error)
		scanDir = func(apath string, dpath string, oldDir *shareDirectory) (*shareDirectory, error) {
			dir := &shareDirectory{
				dirs:      make(map[string]*shareDirectory),
				files:     make(map[string]*shareFile),
				aliasPath: apath,
			}

			files, err := ioutil.ReadDir(dpath)
			if err != nil {
				return nil, err
			}
			for _, file := range files {
				if file.IsDir() {
					subOldDir := func() *shareDirectory {
						if oldDir == nil {
							return nil
						}
						return oldDir.dirs[file.Name()]
					}()
					subdir, err := scanDir(filepath.Join(apath, file.Name()), filepath.Join(dpath, file.Name()), subOldDir)
					if err != nil {
						return nil, err
					}
					dir.dirs[file.Name()] = subdir

				} else {
					var tthl TTHLeaves
					var tth TTH

					aliasPath := filepath.Join(apath, file.Name())
					origPath := filepath.Join(dpath, file.Name())

					// solve symlinks
					realPath, err := filepath.EvalSymlinks(origPath)
					if err != nil {
						return nil, err
					}

					// get real file info
					var finfo os.FileInfo
					finfo, err = os.Stat(realPath)
					if err != nil {
						return nil, err
					}

					fileSize := uint64(finfo.Size())
					fileModTime := finfo.ModTime()

					// recover tth if size and mtime are the same
					if oldDir != nil && oldDir.files[file.Name()] != nil &&
						fileSize == oldDir.files[file.Name()].size &&
						fileModTime.Equal(oldDir.files[file.Name()].modTime) {
						tth = oldDir.files[file.Name()].tth

					} else {
						var err error
						tthl, err = TTHLeavesFromFile(realPath)
						if err != nil {
							return nil, err
						}

						tth = TTHFromLeaves(tthl)
					}

					dir.files[file.Name()] = &shareFile{
						size:      fileSize,
						modTime:   fileModTime,
						tthl:      tthl,
						tth:       tth,
						aliasPath: aliasPath,
						realPath:  realPath,
					}
					dir.size += fileSize
					count += 1
					size += fileSize
				}
			}
			return dir, nil
		}
		for alias, root := range copyRoots {
			oldDir := func() *shareDirectory {
				if t, ok := sm.client.shareTree[alias]; ok {
					return t
				}
				return nil
			}()
			rdir, err := scanDir("/"+alias, root, oldDir)
			if err != nil {
				panic(err)
			}
			tree[alias] = rdir
		}
		return tree, count, size
	}()

	// generate new file list
	fileList, err := func() ([]byte, error) {
		fl := &FileList{
			CID:       dcBase32Encode(sm.client.clientId),
			Generator: sm.client.conf.ListGenerator,
		}

		var scanDir func(dir *shareDirectory) *FileListDirectory
		scanDir = func(dir *shareDirectory) *FileListDirectory {
			fd := &FileListDirectory{}
			for name, file := range dir.files {
				fd.Files = append(fd.Files, &FileListFile{
					Name: name,
					Size: file.size,
					TTH:  file.tth,
				})
			}
			for name, sdir := range dir.dirs {
				sfd := scanDir(sdir)
				sfd.Name = name
				fd.Dirs = append(fd.Dirs, sfd)
			}
			return fd
		}
		for alias, dir := range shareTree {
			fld := scanDir(dir)
			fld.Name = alias
			fl.Dirs = append(fl.Dirs, fld)
		}

		return fl.Export()
	}()
	if err != nil {
		panic(err)
	}

	// compress file list
	fileList, err = func() ([]byte, error) {
		var out bytes.Buffer
		bw, err := bzip2.NewWriter(&out, nil)
		if err != nil {
			return nil, err
		}
		in := bytes.NewReader(fileList)
		if _, err = io.Copy(bw, in); err != nil {
			return nil, err
		}
		bw.Close()
		return out.Bytes(), nil
	}()
	if err != nil {
		panic(err)
	}

	sm.client.Safe(func() {
		// override atomically
		sm.client.shareTree = shareTree
		sm.client.fileList = fileList
		sm.client.shareCount = shareCount
		sm.client.shareSize = shareSize

		// inform hub
		if sm.client.connHub.terminateRequested == false && sm.client.connHub.state == "initialized" {
			sm.client.sendInfos(false)
		}

		if sm.client.OnShareIndexed != nil {
			sm.client.OnShareIndexed()
		}
	})
}

// ShareAdd adds a given directory (dpath) to the client share, with the given
// alias, and starts indexing its subdirectories and files.
// if a directory with the same alias was added previously, it is replaced with
// the new one. OnShareIndexed is called when the indexing is finished.
func (c *Client) ShareAdd(alias string, dpath string) {
	c.shareRoots[alias] = dpath

	// always schedule indexing
	if c.shareIndexer.indexRequested == false {
		c.shareIndexer.indexRequested = true
		c.shareIndexer.indexChan <- struct{}{}
	}
}

// ShareDel removes a directory with the given alias from the client share, and
// starts reindexing the current share.
func (c *Client) ShareDel(alias string) {
	if _, ok := c.shareRoots[alias]; !ok {
		return
	}

	delete(c.shareRoots, alias)

	// always schedule indexing
	if c.shareIndexer.indexRequested == false {
		c.shareIndexer.indexRequested = true
		c.shareIndexer.indexChan <- struct{}{}
	}
}
