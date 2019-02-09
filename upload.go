package dctoolkit

import (
    "fmt"
    "io"
    "os"
    "time"
    "bytes"
    "strings"
    "io/ioutil"
    "path/filepath"
)

var errorNoSlots = fmt.Errorf("no slots available")

type upload struct {
    client          *Client
    state           string
    pconn           *peerConn
    reader          io.ReadCloser
    compressed      bool
    query         string
    start           uint64
    length          uint64
    offset          uint64
    lastPrintTime   time.Time
}

func (*upload) isTransfer() {}

func newUpload(client *Client, pconn *peerConn, msg *msgNmdcAdcGet) error {
    u := &upload{
        client: client,
        state: "transfering",
        pconn: pconn,
    }

    u.query = msg.Query
    u.start = msg.Start
    u.compressed = (client.conf.PeerDisableCompression == false &&
        msg.Compress == true)

    dolog(LevelInfo, "[upload request] %s/%s (s=%d l=%d)",
        pconn.remoteNick, dcReadableQuery(u.query), u.start, msg.Length)

    // check available slots
    if u.client.uploadSlotAvail <= 0 {
        return errorNoSlots
    }

    err := func() error {
        // upload is file list
        if u.query == "file files.xml.bz2" {
            if u.start != 0 || msg.Length != -1 {
                return fmt.Errorf("filelist seeking is not supported")
            }

            u.reader = ioutil.NopCloser(bytes.NewReader(u.client.fileList))
            u.length = uint64(len(u.client.fileList))
            return nil
        }

        // upload is a file by TTH or its tthl
        fpath,tthl := func() (fpath string, tthl []byte) {
            msgTTH := u.query[9:] // skip "file TTH/" or "tthl TTH/"

            var scanDir func(rpath string, dir *shareDirectory) bool
            scanDir = func(rpath string, dir *shareDirectory) bool {
                for fname,file := range dir.files {
                    if file.tth == msgTTH {
                        fpath = filepath.Join(rpath, fname)
                        tthl = file.tthl
                        return true
                    }
                }
                for sname,sdir := range dir.dirs {
                    if scanDir(filepath.Join(rpath, sname), sdir) == true {
                        return true
                    }
                }
                return false
            }
            for _,dir := range u.client.shareTree {
                if scanDir(dir.path, dir.shareDirectory) == true {
                    break
                }
            }
            return
        }()
        if fpath == "" {
            return fmt.Errorf("file does not exists")
        }

        // upload is file tthl
        if strings.HasPrefix(u.query, "tthl TTH") {
            if u.start != 0 || msg.Length != -1 {
                return fmt.Errorf("tthl seeking is not supported")
            }
            u.reader = ioutil.NopCloser(bytes.NewReader(tthl))
            u.length = uint64(len(tthl))
            return nil
        }

        // solve symbolic links
        var err error
        fpath,err = filepath.EvalSymlinks(fpath)
        if err != nil {
            return err
        }

        // get size
        var finfo os.FileInfo
        finfo,err = os.Stat(fpath)
        if err != nil {
            return err
        }

        // open file
        var f *os.File
        f,err = os.Open(fpath)
        if err != nil {
            return err
        }

        // apply start
        _,err = f.Seek(int64(u.start), 0)
        if err != nil {
            f.Close()
            return err
        }

        // set real length
        maxLength := uint64(finfo.Size()) - u.start
        if msg.Length != -1 {
            if uint64(msg.Length) > maxLength {
                f.Close()
                return fmt.Errorf("length too big")
            }
            u.length = uint64(msg.Length)
        } else {
            u.length = maxLength
        }

        u.reader = f
        return nil
    }()
    if err != nil {
        return err
    }

    u.pconn.conn.SendQueued(&msgNmdcAdcSnd{
        Query: u.query,
        Start: u.start,
        Length: u.length,
        Compressed: u.compressed,
    })

    client.transfers[u] = struct{}{}
    u.client.uploadSlotAvail -= 1

    u.client.wg.Add(1)
    go u.do()
    return nil
}

func (u *upload) terminate() {
    switch u.state {
    case "terminated":
        return

    default:
        panic(fmt.Errorf("terminate() unsupported in state '%s'", u.state))
    }
    u.state = "terminated"
    delete(u.client.transfers, u)
    u.pconn.state = "wait_upload"
    u.pconn.wakeUp <- struct{}{}
}

func (u *upload) do() {
    defer u.client.wg.Done()

    err := func() error {
        u.pconn.conn.SetBinaryMode(true)
        if u.compressed == true {
            u.pconn.conn.SetWriteCompression(true)
        }

        var buf [1024 * 1024]byte
        for {
            n,err := u.reader.Read(buf[:])
            if err != nil && err != io.EOF {
                return err
            }
            if n == 0 {
                break
            }

            u.offset += uint64(n)

            err = u.pconn.conn.WriteBinary(buf[:n])
            if err != nil {
                return err
            }

            if time.Since(u.lastPrintTime) >= (1 * time.Second) {
                u.lastPrintTime = time.Now()
                dolog(LevelInfo, "[sent] %d/%d", u.offset, u.length)
            }
        }

        if u.compressed == true {
            u.pconn.conn.SetWriteCompression(false)
        }
        u.pconn.conn.SetBinaryMode(false)

        u.client.Safe(func() {
            u.state = "success"
        })
        return nil
    }()

    u.client.Safe(func() {
        switch u.state {
        case "terminated":

        case "success":
            delete(u.client.transfers, u)
            u.pconn.state = "wait_upload"
            u.pconn.wakeUp <- struct{}{}

        default:
            dolog(LevelInfo, "ERR (upload) [%s]: %s", u.pconn.remoteNick, err)
            delete(u.client.transfers, u)
            u.pconn.state = "wait_upload"
            u.pconn.wakeUp <- struct{}{}
        }

        if u.reader != nil {
            u.reader.Close()
        }

        u.client.uploadSlotAvail += 1

        if u.state == "success" {
            dolog(LevelInfo, "[upload finished] %s/%s (s=%d l=%d)",
                u.pconn.remoteNick, dcReadableQuery(u.query), u.start, u.length)
        } else {
            dolog(LevelInfo, "[upload failed] %s/%s",
                u.pconn.remoteNick, dcReadableQuery(u.query))
        }
    })
}
