package dctoolkit

import (
    "fmt"
    "time"
    "net"
    "regexp"
    "strings"
    "crypto/tls"
)

var reCmdDirection = regexp.MustCompile("^(Download|Upload) ([0-9]+)$")
var reCmdAdcGet = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) (-1|[0-9]+)( ZL1)?$")
var reCmdAdcSnd = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) ([0-9]+)( ZL1)?$")

var errorDelegated = fmt.Errorf("delegated")

type nickDirectionPair struct {
    nick string
    direction string
}

type peerConn struct {
    client              *Client
    tls                 bool
    isActive            bool
    wakeUp              chan struct{}
    state               string
    conn                *protocol
    passiveIp           string
    passivePort         uint
    remoteNick          string
    remoteLock          []byte
    localDirection      string
    localBet            uint
    remoteDirection     string
    remoteBet           uint
    direction           string
}

func newPeerConn(client *Client, tls bool, isActive bool, rawconn net.Conn, ip string, port uint) *peerConn {
    p := &peerConn{
        client: client,
        tls: tls,
        isActive: isActive,
        wakeUp: make(chan struct{}, 1),
    }
    p.client.peerConns[p] = struct{}{}

    securestr := func() string {
        if p.tls {
            return " (secure)"
        }
        return ""
    }()

    if isActive == true {
        dolog(LevelInfo, "[peer incoming] %s%s", connRemoteAddr(rawconn), securestr)
        p.state = "connected"
        p.conn = newprotocol(rawconn, "p", 60 * time.Second, 10 * time.Second)
    } else {
        dolog(LevelInfo, "[peer outgoing] %s:%d%s", ip, port, securestr)
        p.state = "connecting"
        p.passiveIp = ip
        p.passivePort = port
    }

    p.client.wg.Add(1)
    go p.do()
    return p
}

func (p *peerConn) terminate() {
    switch p.state {
    case "terminated":
        return

    case "connecting":
        p.wakeUp <- struct{}{}

    case "connected","mynick","lock","wait_upload","wait_download":
        p.conn.terminate()

    default:
        panic(fmt.Errorf("terminate() unsupported in state '%s'", p.state))
    }
    if p.remoteNick != "" && p.direction != "" {
        delete(p.client.peerConnsByKey, nickDirectionPair{ p.remoteNick, p.direction })
    }
    delete(p.client.peerConns, p)
    p.state = "terminated"
}

func (p *peerConn) do() {
    defer p.client.wg.Done()

    err := func() error {
        if p.state == "connecting" {
            var rawconn net.Conn
            connected := make(chan error, 1)
            go func() {
                var err error
                for i := 0; i < 3; i++ {
                    rawconn,err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", p.passiveIp, p.passivePort), 5 * time.Second)
                    if err == nil {
                        break
                    }
                }
                connected <- err
            }()

            select {
            case <- p.wakeUp:
                return errorTerminated

            case err := <- connected:
                if err != nil {
                    return err
                }
            }

            var err error
            p.client.Safe(func() {
                if p.state == "terminated" {
                    err = errorTerminated
                    return
                }

                securestr := func() string {
                    if p.tls {
                        return " (secure)"
                    }
                    return ""
                }()
                dolog(LevelInfo, "[peer connected] %s%s", connRemoteAddr(rawconn), securestr)
                p.state = "connected"
                if p.tls == true {
                    rawconn = tls.Client(rawconn, &tls.Config{ InsecureSkipVerify: true })
                }
                p.conn = newprotocol(rawconn, "p", 60 * time.Second, 10 * time.Second)
            })
            if err != nil {
                return err
            }

            // if transfer is passive, we are the first to send MyNick and Lock
            p.conn.Send <- msgCommand{ "MyNick", p.client.conf.Nick }
            p.conn.Send <- msgCommand{ "Lock",
                fmt.Sprintf("EXTENDEDPROTOCOLABCABCABCABCABCABC Pk=%sRef=%s:%d",
                p.client.conf.PkValue, p.client.hubSolvedIp, p.client.conf.HubPort) }
        }

        for {
            msg,err := p.conn.Receive()
            if err != nil {
                p.conn.terminate()
                return err
            }

            err = p.handleMessage(msg)
            if err == errorDelegated {
                <- p.wakeUp

            } else if err != nil {
                p.conn.terminate()
                return err
            }
        }
    }()

    p.client.Safe(func() {
        switch p.state {
        case "terminated":

        //case "delegated":
        //    delete(p.client.peerConns, p)

        default:
            dolog(LevelInfo, "ERR (peerConn): %s", err)
            if p.remoteNick != "" && p.direction != "" {
                delete(p.client.peerConnsByKey, nickDirectionPair{ p.remoteNick, p.direction })
            }
            delete(p.client.peerConns, p)
        }
    })
}

func (p *peerConn) handleMessage(rawmsg msgBase) error {
    p.client.mutex.Lock()
    defer p.client.mutex.Unlock()

    if p.state == "terminated" {
        return errorTerminated
    }

    switch msg := rawmsg.(type) {
    case msgCommand:
        switch msg.Key {
            case "MyNick":
                if p.state != "connected" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, p.state)
                }
                p.state = "mynick"
                p.remoteNick = msg.Args

            case "Lock":
                if p.state != "mynick" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, p.state)
                }
                p.state = "lock"
                p.remoteLock = []byte(strings.Split(msg.Args, " ")[0])

                // if transfer is active, wait remote before sending MyNick and Lock
                if p.isActive {
                    p.conn.Send <- msgCommand{ "MyNick", p.client.conf.Nick }
                    p.conn.Send <- msgCommand{ "Lock",
                        fmt.Sprintf("EXTENDEDPROTOCOLABCABCABCABCABCABC Pk=%s", p.client.conf.PkValue),
                    }
                }

                clientSupports := []string{ "MiniSlots", "XmlBZList", "ADCGet", "TTHL", "TTHF" }
                if p.client.conf.PeerDisableCompression == false {
                    clientSupports = append(clientSupports, "ZLIG")
                }
                p.conn.Send <- msgCommand{ "Supports", strings.Join(clientSupports, " ") }

                // check if there's a pending download
                isPendingDownload := func() bool {
                    dl,ok := p.client.activeDownloadsByPeer[p.remoteNick]
                    if ok && dl.state == "waiting_peer" {
                        return true
                    }
                    return false
                }()

                p.localBet = uint(randomInt(1, 0x7FFF))

                // try download
                if isPendingDownload {
                    p.localDirection = "download"
                    p.conn.Send <- msgCommand{ "Direction", fmt.Sprintf("Download %d", p.localBet) }
                // upload
                } else {
                    p.localDirection = "upload"
                    p.conn.Send <- msgCommand{ "Direction", fmt.Sprintf("Upload %d", p.localBet) }
                }

                p.conn.Send <- msgCommand{ "Key", dcComputeKey(p.remoteLock) }

            case "Supports":
                if p.state != "lock" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, p.state)
                }
                p.state = "supports"

            case "Direction":
                if p.state != "supports" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, p.state)
                }
                p.state = "direction"
                args := reCmdDirection.FindStringSubmatch(msg.Args)
                if args == nil {
                    return fmt.Errorf("Cannot parse Direction arguments: %s", msg.Args)
                }
                p.remoteDirection = strings.ToLower(args[1])
                p.remoteBet = atoui(args[2])

            case "Key":
                if p.state != "direction" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, p.state)
                }
                p.state = "key"

                var direction string
                if p.localDirection == "upload" && p.remoteDirection == "download" {
                    direction = "upload"

                } else if p.localDirection == "download" && p.remoteDirection == "upload" {
                    direction = "download"

                } else if p.localDirection == "download" && p.remoteDirection == "download" {
                    // bet win
                    if p.localBet > p.remoteBet {
                        direction = "download"

                    // bet lost
                    } else if p.localBet < p.remoteBet {
                        direction = "upload"

                        // check if there's a pending download
                        isPendingDownload := func() bool {
                            dl,ok := p.client.activeDownloadsByPeer[p.remoteNick]
                            if ok && dl.state == "waiting_peer" {
                                return true
                            }
                            return false
                        }()
                        if isPendingDownload {
                            // request another peer connection
                            if p.client.conf.ModePassive == false {
                                p.client.connectToMe(p.remoteNick)
                            } else {
                                p.client.revConnectToMe(p.remoteNick)
                            }
                        }

                    } else {
                        return fmt.Errorf("equal random numbers")
                    }

                } else {
                    return fmt.Errorf("double upload request")
                }

                key := nickDirectionPair{ p.remoteNick, direction }

                if _,ok := p.client.peerConnsByKey[key]; ok {
                    return fmt.Errorf("a connection with this peer and direction already exists")
                }
                p.client.peerConnsByKey[key] = p

                p.direction = direction

                // upload
                if direction == "upload" {
                    p.state = "wait_upload"

                // download
                } else {
                    p.state = "wait_download"

                    dl := func() *Download {
                        dl,ok := p.client.activeDownloadsByPeer[p.remoteNick]
                        if ok && dl.state == "waiting_peer" {
                            return dl
                        }
                        return nil
                    }()

                    if dl != nil {
                        p.state = "delegated"
                        dl.pconn = p
                        dl.wakeUp <- struct{}{}
                        return errorDelegated
                    }
                }

            case "ADCGET":
                if p.state != "wait_upload" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, p.state)
                }
                args := reCmdAdcGet.FindStringSubmatch(msg.Args)
                if args == nil {
                    return fmt.Errorf("Cannot parse ADCGET args: %s", msg.Args)
                }

                err := newUpload(p.client, p, args)
                if err != nil {
                    dolog(LevelInfo, "cannot start upload: %s", err)

                    if err == errorNoSlots {
                        p.conn.Send <- msgCommand{ "MaxedOut", "" }
                    } else {
                        p.conn.Send <- msgCommand{ "Error", "File Not Available" }
                    }

                } else {
                    p.state = "delegated"
                    return errorDelegated
                }

            default:
                return fmt.Errorf("unhandled: [%s] %s", msg.Key, msg.Args)
        }
    }
    return nil
}
