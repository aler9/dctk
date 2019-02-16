package dctoolkit

import (
    "fmt"
    "time"
    "net"
    "strings"
    "crypto/tls"
)

type nickDirectionPair struct {
    nick string
    direction string
}

type connPeer struct {
    client              *Client
    isEncrypted         bool
    isActive            bool
    wakeUp              chan struct{}
    state               string
    conn                protocol
    adcToken            string
    passiveIp           string
    passivePort         uint
    peer                *Peer
    remoteLock          []byte
    localDirection      string
    localBet            uint
    remoteDirection     string
    remoteBet           uint
    direction           string
    download            *Download
}

func newConnPeer(client *Client, isEncrypted bool, isActive bool,
    rawconn net.Conn, ip string, port uint, adcToken string) *connPeer {
    p := &connPeer{
        client: client,
        isEncrypted: isEncrypted,
        isActive: isActive,
        wakeUp: make(chan struct{}, 1),
        adcToken: adcToken,
    }
    p.client.connPeers[p] = struct{}{}

    securestr := func() string {
        if p.isEncrypted == true {
            return " (secure)"
        }
        return ""
    }()

    if isActive == true {
        dolog(LevelInfo, "[peer incoming] %s%s", connRemoteAddr(rawconn), securestr)
        p.state = "connected"
        if client.protoIsAdc == true {
            p.conn = newProtocolAdc("p", rawconn, true, true)
        } else {
            p.conn = newProtocolNmdc("p", rawconn, true, true)
        }
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

func (p *connPeer) terminate() {
    switch p.state {
    case "terminated":
        return

    case "connecting":
        p.wakeUp <- struct{}{}

    case "connected","mynick","lock","wait_upload","wait_download":
        p.conn.Terminate()

    default:
        panic(fmt.Errorf("terminate() unsupported in state '%s'", p.state))
    }
    if p.peer != nil && p.direction != "" {
        delete(p.client.connPeersByKey, nickDirectionPair{ p.peer.Nick, p.direction })
    }
    delete(p.client.connPeers, p)
    p.state = "terminated"
}

func (p *connPeer) do() {
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
                    if p.isEncrypted == true {
                        return " (secure)"
                    }
                    return ""
                }()
                dolog(LevelInfo, "[peer connected] %s%s", connRemoteAddr(rawconn), securestr)
                p.state = "connected"
                if p.isEncrypted == true {
                    rawconn = tls.Client(rawconn, &tls.Config{ InsecureSkipVerify: true })
                }
                if p.client.protoIsAdc == true {
                    p.conn = newProtocolAdc("p", rawconn, true, true)
                } else {
                    p.conn = newProtocolNmdc("p", rawconn, true, true)
                }
            })
            if err != nil {
                return err
            }

            // if transfer is passive, we are the first to talk
            if p.client.protoIsAdc == true {
                p.conn.Write(&msgAdcCSupports{
                    msgAdcTypeC{},
                    msgAdcKeySupports{ map[string]struct{}{
                        "CSUP": struct{}{},
                        "ADBAS0": struct{}{},
                        "ADBASE": struct{}{},
                        "ADTIGR": struct{}{},
                        "ADBZIP": struct{}{},
                        "ADZLIG": struct{}{},
                    } },
                })

            } else {
                p.conn.Write(&msgNmdcMyNick{ Nick: p.client.conf.Nick })
                p.conn.Write(&msgNmdcLock{ Values: []string{fmt.Sprintf(
                    "EXTENDEDPROTOCOLABCABCABCABCABCABC Pk=%sRef=%s:%d",
                    p.client.conf.PkValue, p.client.hubSolvedIp, p.client.hubPort)} })
            }
        }

        for {
            msg,err := p.conn.Read()
            if err != nil {
                p.conn.Terminate()
                return err
            }

            err = p.handleMessage(msg)
            if err != nil {
                p.conn.Terminate()
                return err
            }
        }
    }()

    p.client.Safe(func() {
        // connection error while downloading
        if p.state == "delegated" && p.direction == "download" {
            p.download.state = "terminated"
            p.download.delegatedError <- errorTerminated
        }

        switch p.state {
        case "terminated":

        default:
            dolog(LevelInfo, "ERR (connPeer): %s", err)
            if p.peer != nil && p.direction != "" {
                delete(p.client.connPeersByKey, nickDirectionPair{ p.peer.Nick, p.direction })
            }
            delete(p.client.connPeers, p)
        }
    })
}

func (p *connPeer) handleMessage(rawmsg msgDecodable) error {
    p.client.mutex.Lock()
    defer p.client.mutex.Unlock()

    if p.state == "terminated" {
        return errorTerminated
    }

    if p.state == "delegated" && p.direction == "download" {
        p.download.onDelegatedMessage(rawmsg)
        return nil
    }

    switch msg := rawmsg.(type) {
    case *msgAdcCStatus:
        if msg.Code != 0 {
            return fmt.Errorf("error (%d): %s", msg.Code, msg.Message)
        }

    case *msgAdcCSupports:
        if p.state != "connected" {
            return fmt.Errorf("[Supports] invalid state: %s", p.state)
        }
        p.state = "supports"
        if p.isActive == true {
            p.conn.Write(&msgAdcCSupports{
                msgAdcTypeC{},
                msgAdcKeySupports{ map[string]struct{}{
                    "CSUP": struct{}{},
                    "ADBAS0": struct{}{},
                    "ADBASE": struct{}{},
                    "ADTIGR": struct{}{},
                    "ADBZIP": struct{}{},
                    "ADZLIG": struct{}{},
                } },
            })
        } else {
            p.conn.Write(&msgAdcCInfos{
                msgAdcTypeC{},
                msgAdcKeyInfos{ map[string]string{
                    adcFieldClientId: dcBase32Encode(p.client.clientId),
                    adcFieldToken: p.adcToken,
                } },
            })
        }

    case *msgAdcCInfos:
        if p.state != "supports" {
            return fmt.Errorf("[Infos] invalid state: %s", p.state)
        }
        p.state = "infos"
        if p.isActive == true {
            p.conn.Write(&msgAdcCInfos{
                msgAdcTypeC{},
                msgAdcKeyInfos{ map[string]string{ adcFieldClientId: dcBase32Encode(p.client.clientId) } },
            })
        }

    case *msgNmdcMyNick:
        if p.state != "connected" {
            return fmt.Errorf("[MyNick] invalid state: %s", p.state)
        }
        p.state = "mynick"
        p.peer = p.client.peerByNick(msg.Nick)
        if p.peer == nil {
            return fmt.Errorf("peer not connected to hub (%s)", msg.Nick)
        }

    case *msgNmdcLock:
        if p.state != "mynick" {
            return fmt.Errorf("[Lock] invalid state: %s", p.state)
        }
        p.state = "lock"
        p.remoteLock = []byte(msg.Values[0])

        // if transfer is active, wait remote before sending MyNick and Lock
        if p.isActive {
            p.conn.Write(&msgNmdcMyNick{ Nick: p.client.conf.Nick })
            p.conn.Write(&msgNmdcLock{ Values: []string{ fmt.Sprintf(
                "EXTENDEDPROTOCOLABCABCABCABCABCABC Pk=%s", p.client.conf.PkValue) } })
        }

        clientSupports := []string{ "MiniSlots", "XmlBZList", "ADCGet", "TTHL", "TTHF" }
        if p.client.conf.PeerDisableCompression == false {
            clientSupports = append(clientSupports, "ZLIG")
        }
        p.conn.Write(&msgNmdcSupports{ Features: clientSupports })

        // check if there's a pending download
        isPendingDownload := func() bool {
            dl,ok := p.client.activeDownloadsByPeer[p.peer.Nick]
            if ok && dl.state == "waiting_peer" {
                return true
            }
            return false
        }()

        p.localBet = uint(randomInt(1, 0x7FFF))

        // try download
        if isPendingDownload {
            p.localDirection = "download"
            p.conn.Write(&msgNmdcDirection{
                Direction: "Download",
                Bet: p.localBet,
            })
        // upload
        } else {
            p.localDirection = "upload"
            p.conn.Write(&msgNmdcDirection{
                Direction: "Upload",
                Bet: p.localBet,
            })
        }

        p.conn.Write(&msgNmdcKey{ Key: nmdcComputeKey(p.remoteLock) })

    case *msgNmdcSupports:
        if p.state != "lock" {
            return fmt.Errorf("[Supports] invalid state: %s", p.state)
        }
        p.state = "supports"

    case *msgNmdcDirection:
        if p.state != "supports" {
            return fmt.Errorf("[Direction] invalid state: %s", p.state)
        }
        p.state = "direction"
        p.remoteDirection = strings.ToLower(msg.Direction)
        p.remoteBet = msg.Bet

    case *msgNmdcKey:
        if p.state != "direction" {
            return fmt.Errorf("[Key] invalid state: %s", p.state)
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
                    dl,ok := p.client.activeDownloadsByPeer[p.peer.Nick]
                    if ok && dl.state == "waiting_peer" {
                        return true
                    }
                    return false
                }()
                if isPendingDownload == true {
                    // request another peer connection
                    peer := p.client.peerByNick(p.peer.Nick)
                    if peer != nil {
                        p.client.peerRequestConnection(peer)
                    }
                }

            } else {
                return fmt.Errorf("equal random numbers")
            }

        } else {
            return fmt.Errorf("double upload request")
        }

        key := nickDirectionPair{ p.peer.Nick, direction }

        if _,ok := p.client.connPeersByKey[key]; ok {
            return fmt.Errorf("a connection with this peer and direction already exists")
        }
        p.client.connPeersByKey[key] = p

        p.direction = direction

        // upload
        if p.direction == "upload" {
            p.state = "wait_upload"

        // download
        } else {
            p.state = "wait_download"

            dl := func() *Download {
                dl,ok := p.client.activeDownloadsByPeer[p.peer.Nick]
                if ok && dl.state == "waiting_peer" {
                    return dl
                }
                return nil
            }()

            if dl != nil {
                p.state = "delegated"
                p.download = dl
                dl.pconn = p
                dl.wakeUp <- struct{}{}
            }
        }

    case *msgNmdcAdcGet:
        if p.state != "wait_upload" {
            return fmt.Errorf("[AdcGet] invalid state: %s", p.state)
        }

        err := newUpload(p.client, p, msg)
        if err != nil {
            dolog(LevelInfo, "cannot start upload: %s", err)

            if err == errorNoSlots {
                p.conn.Write(&msgNmdcMaxedOut{})
            } else {
                p.conn.Write(&msgNmdcError{ Error: "File Not Available" })
            }

        } else {
            p.state = "delegated"
        }

    default:
        return fmt.Errorf("unhandled: %T %+v", rawmsg, rawmsg)
    }
    return nil
}
