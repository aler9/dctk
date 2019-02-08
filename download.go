package dctoolkit

import (
    "fmt"
    "time"
    "bytes"
    "io/ioutil"
    "compress/bzip2"
)

type DownloadConf struct {
    Nick            string
    TTH             string
    Start           uint64
    Length          int64
    SkipValidation  bool
    filelist        bool
}

type Download struct {
    conf                DownloadConf
    client              *Client
    state               string
    wakeUp              chan struct{}
    pconn               *peerConn
    compressed          bool
    query               string
    content             []byte
    length              uint64
    offset              uint64
    lastPrintTime       time.Time
}

func (*Download) isTransfer() {}

func (client *Client) DownloadFileList(nick string) (*Download,error) {
    return client.Download(DownloadConf{
        Nick: nick,
        filelist: true,
    })
}

func (client *Client) Download(conf DownloadConf) (*Download,error) {
    if conf.Length <= 0 {
        conf.Length = -1
    }
    if conf.filelist == false && TTHIsValid(conf.TTH) == false {
        return nil, fmt.Errorf("invalid TTH")
    }

    d := &Download{
        conf: conf,
        client: client,
        wakeUp: make(chan struct{}, 1),
        state: "uninitialized",
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
        d.conf.Nick, dcReadableRequest(d.query), d.conf.Start, d.conf.Length)

    d.client.wg.Add(1)
    go d.do()
    return d, nil
}

func (d *Download) Conf() DownloadConf {
    return d.conf
}

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
        d.pconn.wakeUp <- struct{}{}
    }
}

func (d *Download) Terminate() {
    d.terminate()
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
                        if _,ok := d.client.activeDownloadsByPeer[d.conf.Nick]; ok {
                            return "wait"
                        }

                    case "waiting_activedl":
                        d.state = "waiting_slot"
                        if d.client.downloadSlotAvail <= 0 {
                            return "wait"
                        }

                    case "waiting_slot":
                        d.state = "waiting_peer"
                        d.client.activeDownloadsByPeer[d.conf.Nick] = d
                        d.client.downloadSlotAvail -= 1

                        if _,ok := d.client.peerConnsByKey[nickDirectionPair{ d.conf.Nick, "download" }]; !ok {
                            // request peer connection
                            if d.client.conf.ModePassive == false {
                                d.client.connectToMe(d.conf.Nick)
                            } else {
                                d.client.revConnectToMe(d.conf.Nick)
                            }
                            return "wait_timed"
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

        for {
            msg,err := d.pconn.conn.Receive()
            if err != nil {
                return err
            }

            err = d.handleMessage(msg)
            if err != nil {
                return err
            }
        }
    }()

    d.client.Safe(func() {
        switch d.state {
        case "terminated":

        case "success":
            delete(d.client.transfers, d)
            d.pconn.state = "wait_download"
            d.pconn.wakeUp <- struct{}{}

        default:
            dolog(LevelInfo, "ERR (download) [%s]: %s", d.conf.Nick, err)
            delete(d.client.transfers, d)
            if d.pconn != nil {
                d.pconn.state = "wait_download"
                d.pconn.wakeUp <- struct{}{}
            }
        }

        if d.client.activeDownloadsByPeer[d.conf.Nick] == d {
            delete(d.client.activeDownloadsByPeer, d.conf.Nick)
        }

        d.client.downloadSlotAvail += 1

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
                if (od.state == "waiting_activedl" && d.conf.Nick == od.conf.Nick) {
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
                d.conf.Nick, dcReadableRequest(d.query), d.conf.Start, len(d.content))
            if d.client.OnDownloadSuccessful != nil {
                d.client.OnDownloadSuccessful(d)
            }
        } else {
            dolog(LevelInfo, "[download failed] %s/%s", d.conf.Nick, dcReadableRequest(d.query))
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

    d.pconn.conn.Send <- msgCommand{ "ADCGET", fmt.Sprintf("%s %d %d%s",
        d.query,
        d.conf.Start,
        d.conf.Length,
        func() string {
            if requestCompressed == true {
                return " ZL1"
            }
            return ""
        }()),
    }
}

func (d *Download) handleMessage(rawmsg msgBase) error {
    d.client.mutex.Lock()
    defer d.client.mutex.Unlock()

    if d.state == "terminated" {
        return errorTerminated
    }

    switch msg := rawmsg.(type) {
    case msgCommand:
        switch msg.Key {
        case "MaxedOut":
            return fmt.Errorf("maxed out")

        case "Error":
            return fmt.Errorf("error: %s", msg.Args)

        case "ADCSND":
            args := reCmdAdcSnd.FindStringSubmatch(msg.Args)
            if args == nil {
                return fmt.Errorf("Cannot parse ADCSND args: %s", msg.Args)
            }

            d.state = "transfering"

            replyQuery := args[1]
            if replyQuery != d.query {
                return fmt.Errorf("filename returned by client is wrong: %s vs %s", replyQuery, d.query)
            }
            replyStart := atoui64(args[4])
            if replyStart != d.conf.Start {
                return fmt.Errorf("peer returned wrong start: %d instead of %d", replyStart, d.conf.Start)
            }
            replyLength := atoui64(args[5])
            d.compressed = (args[6] != "")
            if d.compressed == true && d.client.conf.PeerDisableCompression == true {
                return fmt.Errorf("compression is active but is disabled")
            }

            if d.conf.Length == -1 {
                d.length = replyLength
            } else {
                d.length = uint64(d.conf.Length)
                if d.length != replyLength {
                    return fmt.Errorf("peer returned wrong length: %d instead of %d", d.length, replyLength)
                }
            }

            if d.length == 0 {
                return fmt.Errorf("downloading null files is not supported")
            }

            d.content = make([]byte, d.length)
            d.pconn.conn.SetBinaryMode(true)
            if d.compressed == true {
                d.pconn.conn.SetReadCompressionOn()
            }

        default:
            return fmt.Errorf("unhandled: [%s] %s", msg.Key, msg.Args)
        }

    case msgBinary:
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
            d.pconn.conn.SetBinaryMode(false)

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
    }
    return nil
}
