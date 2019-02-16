package dctoolkit

import (
    "fmt"
    "time"
    "bytes"
    "io/ioutil"
    "compress/bzip2"
)

type DownloadConf struct {
    // the peer you want to download from
    Peer            *Peer
    // the TTH of the file you want to download
    TTH             string
    // the starting point of the file part you want to download, in bytes
    Start           uint64
    // the length of the file part. Leave zero if you want to download the entire file
    Length          int64
    // after download, do not attempt to validate the file through its TTH
    SkipValidation  bool
    filelist        bool
}

type Download struct {
    conf                DownloadConf
    client              *Client
    state               string
    wakeUp              chan struct{}
    pconn               *connPeer
    query               string
    content             []byte
    length              uint64
    offset              uint64
    lastPrintTime       time.Time
    delegatedError      chan error
}

func (*Download) isTransfer() {}

// DownloadFileList starts downloading the file list of a given peer.
func (c *Client) DownloadFileList(peer *Peer) (*Download,error) {
    return c.DownloadFile(DownloadConf{
        Peer: peer,
        filelist: true,
    })
}

// DownloadFLFile starts downloading a file given a file list entry.
func (c *Client) DownloadFLFile(peer *Peer, file *FileListFile) (*Download,error) {
    return c.DownloadFile(DownloadConf{
        Peer: peer,
        TTH: file.TTH,
    })
}

// DownloadFLDirectory starts downloading recursively all the files
// inside a file list directory.
func (c *Client) DownloadFLDirectory(peer *Peer, dir *FileListDirectory) error {
    var dlDir func(sdir *FileListDirectory) error
    dlDir = func(sdir *FileListDirectory) error {
        for _,file := range sdir.Files {
            _,err := c.DownloadFLFile(peer, file)
            if err != nil {
                return err
            }
        }
        for _,ssdir := range sdir.Dirs {
            err := dlDir(ssdir)
            if err != nil {
                return err
            }
        }
        return nil
    }
    return dlDir(dir)
}

// Download starts downloading a file by its Tiger Tree Hash (TTH). See DownloadConf for the options.
func (c *Client) DownloadFile(conf DownloadConf) (*Download,error) {
    if conf.Length <= 0 {
        conf.Length = -1
    }
    if conf.filelist == false && TTHIsValid(conf.TTH) == false {
        return nil, fmt.Errorf("invalid TTH")
    }

    d := &Download{
        conf: conf,
        client: c,
        wakeUp: make(chan struct{}, 1),
        state: "uninitialized",
        delegatedError: make(chan error),
    }
    d.client.transfers[d] = struct{}{}

    // build query
    d.query = func() string {
        if d.conf.filelist == true {
            return "file files.xml.bz2"
        }
        return "file TTH/" + d.conf.TTH
    }()

    dolog(LevelInfo, "[download request] %s/%s (s=%d l=%d)",
        d.conf.Peer.Nick, dcReadableQuery(d.query), d.conf.Start, d.conf.Length)

    d.client.wg.Add(1)
    go d.do()
    return d, nil
}

// Conf returns the configuration passed at download initialization.
func (d *Download) Conf() DownloadConf {
    return d.conf
}

// Content returns the downloaded file content.
func (d *Download) Content() []byte {
    return d.content
}

func (d *Download) terminate() {
    switch d.state {
    case "terminated":
        return

    case "waiting_activedl":
        d.wakeUp <- struct{}{}

    default:
        panic(fmt.Errorf("Terminate() unsupported in state '%s'", d.state))
    }
    d.state = "terminated"
    delete(d.client.transfers, d)
    if d.pconn != nil {
        d.pconn.state = "wait_download"
        d.delegatedError <- errorTerminated
    }
}

func (d *Download) do() {
    defer d.client.wg.Done()

    err := func() error {
        for {
            action := func() (string) {
                d.client.mutex.Lock()
                defer d.client.mutex.Unlock()

                if d.state == "terminated" {
                    return "exit"
                }

                for {
                    switch d.state {
                    case "uninitialized":
                        d.state = "waiting_activedl"
                        if _,ok := d.client.activeDownloadsByPeer[d.conf.Peer.Nick]; ok {
                            return "wait"
                        }

                    case "waiting_activedl":
                        d.state = "waiting_slot"
                        if d.client.downloadSlotAvail <= 0 {
                            return "wait"
                        }

                    case "waiting_slot":
                        d.state = "waiting_peer"
                        d.client.activeDownloadsByPeer[d.conf.Peer.Nick] = d
                        d.client.downloadSlotAvail -= 1

                        if pconn,ok := d.client.connPeersByKey[nickDirectionPair{ d.conf.Peer.Nick, "download" }]; !ok {
                            // request peer connection
                            d.client.peerRequestConnection(d.conf.Peer)
                            return "wait_timed"
                        } else {
                            d.pconn = pconn
                            pconn.state = "delegated"
                            pconn.download = d
                        }

                    default: // waiting_peer"
                        d.state = "connected"
                        return "break"
                    }
                }
            }()
            if action == "exit" {
                return errorTerminated
            }
            if action == "break" {
                break
            }

            if action == "wait_timed" {
                timeout := time.NewTimer(10 * time.Second)
                select {
                case <- timeout.C:
                    return fmt.Errorf("download timed out")
                case <- d.wakeUp:
                }
            } else {
                <- d.wakeUp
            }
        }

        d.requestFile()

        return <- d.delegatedError
    }()

    d.client.Safe(func() {
        switch d.state {
        case "terminated":

        case "success":
            delete(d.client.transfers, d)

        default:
            dolog(LevelInfo, "ERR (download) [%s]: %s", d.conf.Peer.Nick, err)
            delete(d.client.transfers, d)
        }

        if d.client.activeDownloadsByPeer[d.conf.Peer.Nick] == d {
            delete(d.client.activeDownloadsByPeer, d.conf.Peer.Nick)
        }

        if d.state != "waiting_slot" {
            d.client.downloadSlotAvail += 1
        }

        var toWakeUp1 *Download
        var toWakeUp2 *Download

        // unlock another download from the same peer
        for rot,_ := range d.client.transfers {
            if od,ok := rot.(*Download); ok {
                if od.state == "waiting_slot" {
                    toWakeUp1 = od
                    break
                }
            }
        }
        for rot,_ := range d.client.transfers {
            if od,ok := rot.(*Download); ok {
                if (od.state == "waiting_activedl" && d.conf.Peer == od.conf.Peer) {
                    toWakeUp2 = od
                    break
                }
            }
        }

        // send wakeUp once
        if toWakeUp1 != nil {
            toWakeUp1.wakeUp <- struct{}{}
        }
        if toWakeUp2 != nil && toWakeUp1 != toWakeUp2 {
            toWakeUp2.wakeUp <- struct{}{}
        }

        // call callbacks once the procedure has terminated
        if d.state == "success" {
            dolog(LevelInfo, "[download finished] %s/%s (s=%d l=%d)",
                d.conf.Peer.Nick, dcReadableQuery(d.query), d.conf.Start, len(d.content))
            if d.client.OnDownloadSuccessful != nil {
                d.client.OnDownloadSuccessful(d)
            }
        } else {
            dolog(LevelInfo, "[download failed] %s/%s", d.conf.Peer.Nick, dcReadableQuery(d.query))
            if d.client.OnDownloadError != nil {
                d.client.OnDownloadError(d)
            }
        }
    })
}

func (d *Download) requestFile() {
    // activate compression only if file has a minimum size
    requestCompressed := (d.client.conf.PeerDisableCompression == false &&
        (d.conf.Length <= 0 || d.conf.Length >= (1024 * 10)))
    d.state = "request_file"

    d.pconn.conn.Write(&msgNmdcAdcGet{
        Query: d.query,
        Start: d.conf.Start,
        Length: d.conf.Length,
        Compress: requestCompressed,
    })
}

func (d *Download) onDelegatedMessage(rawmsg msgDecodable) {
    err := func() error {
        switch msg := rawmsg.(type) {
        case *msgNmdcMaxedOut:
            return fmt.Errorf("maxed out")

        case *msgNmdcError:
            return fmt.Errorf("error: %s", msg.Error)

        case *msgNmdcAdcSnd:
            d.state = "transfering"

            if msg.Query != d.query {
                return fmt.Errorf("filename returned by client is wrong: %s vs %s", msg.Query, d.query)
            }
            if msg.Start != d.conf.Start {
                return fmt.Errorf("peer returned wrong start: %d instead of %d", msg.Start, d.conf.Start)
            }
            if msg.Compressed == true && d.client.conf.PeerDisableCompression == true {
                return fmt.Errorf("compression is active but is disabled")
            }

            if d.conf.Length == -1 {
                d.length = msg.Length
            } else {
                d.length = uint64(d.conf.Length)
                if d.length != msg.Length {
                    return fmt.Errorf("peer returned wrong length: %d instead of %d", d.length, msg.Length)
                }
            }

            if d.length == 0 {
                return fmt.Errorf("downloading null files is not supported")
            }

            d.content = make([]byte, d.length)
            d.pconn.conn.SetReadBinary(true)
            if msg.Compressed == true {
                d.pconn.conn.SetReadCompressionOn()
            }

        case *msgNmdcBinary:
            newLength := d.offset + uint64(len(msg.Content))
            if newLength > d.length {
                return fmt.Errorf("binary content too long (%d)", newLength)
            }

            copied := copy(d.content[d.offset:], msg.Content)
            d.offset += uint64(copied)

            if time.Since(d.lastPrintTime) >= (1 * time.Second) {
                d.lastPrintTime = time.Now()
                dolog(LevelInfo, "[recv] %d/%d", d.offset, d.length)
            }

            if d.offset == d.length {
                d.pconn.conn.SetReadBinary(false)

                // file list: unzip
                if d.conf.filelist {
                    cnt,err := ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(d.content)))
                    if err != nil {
                        return err
                    }
                    d.content = cnt

                // normal file
                } else {
                    if d.conf.SkipValidation == false && d.conf.Start == 0 && d.conf.Length <= 0 {
                        dolog(LevelInfo, "[download validating]")
                        contentTTH := TTHFromBytes(d.content)
                        if contentTTH != d.conf.TTH {
                            return fmt.Errorf("validation failed")
                        }
                    }
                }

                d.state = "success"
                return errorTerminated
            }

        default:
            return fmt.Errorf("unhandled: %T %+v", rawmsg, rawmsg)
        }
        return nil
    }()

    if err != nil {
        d.pconn.state = "wait_download"
        d.pconn.download = nil
        d.delegatedError <- err
    }
}
