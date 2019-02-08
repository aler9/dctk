package dctoolkit

import (
    "fmt"
    "time"
    "bytes"
    "os"
    "io"
    "io/ioutil"
    "path/filepath"
    "github.com/dsnet/compress/bzip2"
)

type shareFile struct {
    size        uint64
    modTime     time.Time
    tth         string
    tthl        []byte
}

type shareDirectory struct {
    dirs        map[string]*shareDirectory
    files       map[string]*shareFile
}

type shareRootDirectory struct {
    *shareDirectory
    path        string
}

type shareIndexer struct {
    client              *Client
    state               string
    wakeUp              chan struct{}
    indexingRequested   bool
}

func newshareIndexer(client *Client) error {
    client.shareIndexer = &shareIndexer{
        client: client,
        state: "running",
        wakeUp: make(chan struct{}, 1),
    }
    client.shareIndexer.index()
    return nil
}

func (sm *shareIndexer) index() {
    copyRoots := make(map[string]string)
    sm.client.Safe(func() {
        // disable flag
        sm.indexingRequested = false

        // create a copy of shareRoots
        for k,v := range sm.client.shareRoots {
            copyRoots[k] = v
        }
    })

    // generate new tree
    shareTree := func() (tree map[string]*shareRootDirectory) {
        tree = make(map[string]*shareRootDirectory)
        var scanDir func(dpath string, oldDir *shareDirectory) (*shareDirectory,error)
        scanDir = func(dpath string, oldDir *shareDirectory) (*shareDirectory,error) {
            dir := &shareDirectory{
                dirs: make(map[string]*shareDirectory),
                files: make(map[string]*shareFile),
            }

            files,err := ioutil.ReadDir(dpath)
        	if err != nil {
                return nil,err
        	}
        	for _,file := range files {
                if file.IsDir() {
                    subOldDir := func() *shareDirectory {
                        if oldDir == nil {
                            return nil
                        }
                        return oldDir.dirs[file.Name()]
                    }()
                    subdir,err := scanDir(filepath.Join(dpath, file.Name()), subOldDir)
                    if err != nil {
                        return nil,err
                    }
                    dir.dirs[file.Name()] = subdir

                } else {
                    var tthl []byte
                    var tth string

                    // solve symlinks
                    fpath,err := filepath.EvalSymlinks(filepath.Join(dpath, file.Name()))
                    if err != nil {
                        return nil,err
                    }

                    // get real file info
                    var finfo os.FileInfo
                    finfo,err = os.Stat(fpath)
                    if err != nil {
                        return nil,err
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
                        tthl,err = TTHLeavesFromFile(filepath.Join(dpath, file.Name()))
                        if err != nil {
                            return nil,err
                        }

                        tth = TTHFromLeaves(tthl)
                    }

                    dir.files[file.Name()] = &shareFile{
                        size: fileSize,
                        modTime: fileModTime,
                        tthl: tthl,
                        tth: tth,
                    }
                }
        	}
            return dir,nil
        }
        for alias,root := range copyRoots {
            oldDir := func() *shareDirectory {
                if t,ok := sm.client.shareTree[alias]; ok {
                    return t.shareDirectory
                }
                return nil
            }()
            rdir,err := scanDir(root, oldDir)
            if err != nil {
                panic(err)
            }
            tree[alias] = &shareRootDirectory{
                shareDirectory: rdir,
                path: root,
            }
        }
        return
    }()

    // generate new file list
    fileList,err := func() ([]byte,error) {
        fl := &FileList{
            CID: sm.client.clientId,
            Generator: sm.client.conf.ListGenerator,
        }

        var scanDir func(path string, dir *shareDirectory) *FileListDirectory
        scanDir = func(path string, dir *shareDirectory) *FileListDirectory {
            fd := &FileListDirectory{}
            for name,sdir := range dir.dirs {
                sfd := scanDir(filepath.Join(path, name), sdir)
                sfd.Name = name
                fd.Dirs = append(fd.Dirs, sfd)
            }
            for name,file := range dir.files {
                fd.Files = append(fd.Files, &FileListFile{
                    Name: name,
                    Size: file.size,
                    TTH: file.tth,
                })
            }
            return fd
        }
        for alias,dir := range shareTree {
            fld := scanDir(dir.path, dir.shareDirectory)
            fld.Name = alias
            fl.Dirs = append(fl.Dirs, fld)
        }

        return fl.Export()
    }()
    if err != nil {
        panic(err)
    }

    // compress file list
    fileList,err = func() ([]byte,error) {
        var out bytes.Buffer
        bw,err := bzip2.NewWriter(&out, nil)
        if err != nil {
            return nil,err
        }
        in := bytes.NewReader(fileList)
        if _,err = io.Copy(bw, in); err != nil {
            return nil,err
        }
        bw.Close()
        return out.Bytes(), nil
    }()
    if err != nil {
        panic(err)
    }

    // compute share size
    shareSize := func() (ret uint64) {
        var scanDir func(dir *shareDirectory)
        scanDir = func(dir *shareDirectory) {
            for _,f := range dir.files {
                ret += f.size
            }
            for _,d := range dir.dirs {
                scanDir(d)
            }
        }
        for _,d := range shareTree {
            scanDir(d.shareDirectory)
        }
        return
    }()

    sm.client.Safe(func() {
        // override atomically
        sm.client.shareTree = shareTree
        sm.client.fileList = fileList
        sm.client.shareSize = shareSize

        // inform hub
        if sm.client.hubConn != nil && sm.client.hubConn.state == "initialized" {
            sm.client.myInfo()
        }

        if sm.client.OnShareIndexed != nil {
            sm.client.OnShareIndexed()
        }
    })
}

func (sm *shareIndexer) do() {
    defer sm.client.wg.Done()

    for {
        // wait for wake up
        <- sm.wakeUp

        exit := false
        sm.client.Safe(func() {
            if sm.state == "terminated" {
                exit = true
                return
            }
        })
        if exit {
            break
        }

        sm.index()
    }
}

func (sm *shareIndexer) terminate() {
    switch sm.state {
    case "terminated":
        return

    case "running":
        sm.wakeUp <- struct{}{}

    default:
        panic(fmt.Errorf("Terminate() unsupported in state '%s'", sm.state))
    }
    sm.state = "terminated"
}

func (client *Client) ShareAdd(alias string, root string) {
    client.shareRoots[alias] = root

    // always schedule indexing
    if client.shareIndexer.indexingRequested == false {
        client.shareIndexer.indexingRequested = true
        client.shareIndexer.wakeUp <- struct{}{}
    }
}
