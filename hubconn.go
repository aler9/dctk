package dctoolkit

import (
    "fmt"
    "strings"
    "net"
    "time"
    "regexp"
)

var reCmdInfo = regexp.MustCompile("^\\$ALL ("+reStrNick+") (.*?) ?\\$ \\$(.*?)(.)\\$(.*?)\\$([0-9]+)\\$$")
var reCmdConnectToMe = regexp.MustCompile("^("+reStrNick+") ("+reStrIp+"):("+reStrPort+")(S?)$")
var reCmdRevConnectToMe = regexp.MustCompile("^("+reStrNick+") ("+reStrNick+")$")
var reCmdUserIP = regexp.MustCompile("^("+reStrNick+") ("+reStrIp+")$")
var reCmdUserCommand = regexp.MustCompile("^([0-9]+) ([0-9]{1,2}) (.*?)$")

type Peer struct {
    Nick            string
    Description     string
    protocol        string
    Status          byte
    Email           string
    ShareSize       uint64
    Ip              string
    IsOperator      bool
    IsBot           bool
}

func (p *Peer) supportTls() bool {
    // we check only for bit 4
    return (p.Status & (0x01 << 4)) == (0x01 << 4)
}

type hubConn struct {
    client          *Client
    state           string
    conn            *protocol
    uniqueCmds      map[string]struct{}
    myInfoReceived  bool
    peers           map[string]*Peer
}

func newHub(client *Client) error {
    client.hubConn = &hubConn{
        client: client,
        state: "uninitialized",
        uniqueCmds: make(map[string]struct{}),
        peers: make(map[string]*Peer),
    }
    return nil
}

func (c *Client) HubConnect() {
    if c.hubConn.state != "uninitialized" {
        return
    }

    c.hubConn.state = "connecting"
    c.wg.Add(1)
    go c.hubConn.do()
}

func (h *hubConn) terminate() {
    switch h.state {
    case "terminated":
        return

    case "initialized":
        h.conn.terminate()

    default:
        panic(fmt.Errorf("Terminate() unsupported in state '%s'", h.state))
    }
    h.state = "terminated"
}

func (h *hubConn) do() {
    defer h.client.wg.Done()

    err := func() error {
        var rawconn net.Conn
        var err error
        for i := uint(0); i < h.client.conf.HubConnTries; i++ {
            if i > 0 {
                dolog(LevelInfo, "retrying... (%s)", err)
            }

            var ips []net.IP
            ips,err = net.LookupIP(h.client.conf.HubAddress)
            if err != nil {
                break
            }

            h.client.hubSolvedIp = ips[0].String()
            rawconn,err = net.DialTimeout("tcp",
                fmt.Sprintf("%s:%d", h.client.hubSolvedIp, h.client.conf.HubPort), 10 * time.Second)
            if err == nil {
                break
            }
        }
        if err != nil {
            return err
        }

        h.client.Safe(func() {
            dolog(LevelInfo, "[hub connected] %s:%d", h.client.hubSolvedIp, h.client.conf.HubPort)
            h.state = "connected"

            // do not use read timeout since hub does not send data continuously
            h.conn = newprotocol(rawconn, "h", 0, 10 * time.Second)

            // unfortunately chat messages can be sent immediately
            h.conn.ChatAllowed = true
        })

        // activate TCP keepalive
        if err := rawconn.(*net.TCPConn).SetKeepAlive(true); err != nil {
            return err
        }
        if err := rawconn.(*net.TCPConn).SetKeepAlivePeriod(60 * time.Second); err != nil {
            return err
        }

        for {
            msg,err := h.conn.Receive()
            if err != nil {
                h.conn.terminate()
                return err
            }

            err = h.handleMessage(msg)
            if err != nil {
                h.conn.terminate()
                return err
            }
        }
    }()

    h.client.Safe(func() {
        switch h.state {
        case "terminated":

        default:
            h.state = "terminated"
            dolog(LevelInfo, "ERR: %s", err)
            if h.client.OnHubError != nil {
                h.client.OnHubError(err)
            }
        }

        dolog(LevelInfo, "[hub disconnected]")

        // close client too
        h.client.Terminate()
    })
}

func (h *hubConn) handleMessage(rawmsg msgBase) error {
    h.client.mutex.Lock()
    defer h.client.mutex.Unlock()

    if h.state == "terminated" {
        return errorTerminated
    }

    switch msg := rawmsg.(type) {
    case msgCommand:
        switch(msg.Key) {
            case "Lock":
                if h.state != "connected" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                h.state = "lock"

                // https://web.archive.org/web/20150323114734/http://wiki.gusari.org/index.php?title=$Supports
                // https://github.com/eiskaltdcpp/eiskaltdcpp/blob/master/dcpp/Nmdchub.cpp#L618
                hubSupports := []string{ "UserCommand", "NoGetINFO", "NoHello", "UserIP2", "TTHSearch" }
                if h.client.conf.HubDisableCompression == false {
                    hubSupports = append(hubSupports, "ZPipe0")
                }
                // this must be provided, otherwise the final S is stripped from connectToMe
                if h.client.conf.PeerEncryptionMode != DisableEncryption {
                    hubSupports = append(hubSupports, "TLS")
                }

                lock := []byte(strings.Split(msg.Args, " ")[0])
                h.conn.Send <- msgCommand{ "Supports", strings.Join(hubSupports, " ") }
                h.conn.Send <- msgCommand{ "Key", dcComputeKey(lock) }
                h.conn.Send <- msgCommand{ "ValidateNick", h.client.conf.Nick }

            case "Supports":
                if h.state != "lock" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                h.state = "preinitialized"

            // flexhub send HubName just after lock
            // HubName can also be sent twice
            case "HubName":
                if h.state != "preinitialized" && h.state != "lock" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }

            case "ZOn":
                if h.state != "initialized" && h.state != "preinitialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                if h.client.conf.HubDisableCompression == true {
                    return fmt.Errorf("zlib requested but zlib is disabled")
                }
                if err := h.conn.SetReadCompressionOn(); err != nil {
                    return err
                }

            case "HubTopic":
                if h.state != "preinitialized" && h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                if _,ok := h.uniqueCmds[msg.Key]; ok {
                    return fmt.Errorf("%s sent twice", msg.Key)
                }
                h.uniqueCmds[msg.Key] = struct{}{}

            case "GetPass":
                if h.state != "preinitialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                h.conn.Send <- msgCommand{ "MyPass", h.client.conf.Password }
                if _,ok := h.uniqueCmds[msg.Key]; ok {
                    return fmt.Errorf("%s sent twice", msg.Key)
                }
                h.uniqueCmds[msg.Key] = struct{}{}

            case "LogedIn":
                if h.state != "preinitialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                if _,ok := h.uniqueCmds[msg.Key]; ok {
                    return fmt.Errorf("%s sent twice", msg.Key)
                }
                h.uniqueCmds[msg.Key] = struct{}{}

            case "Hello":
                if h.state != "preinitialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                if _,ok := h.uniqueCmds[msg.Key]; ok {
                    return fmt.Errorf("%s sent twice", msg.Key)
                }
                h.uniqueCmds[msg.Key] = struct{}{}

                // The last version of the Neo-Modus client was 1.0091 and is what is commonly used by current clients
                // https://github.com/eiskaltdcpp/eiskaltdcpp/blob/1e72256ac5e8fe6735f81bfbc3f9d90514ada578/dcpp/NmdcHub.h#L119
                h.conn.Send <- msgCommand{ "Version", "1,0091" }
                h.client.myInfo()
                h.conn.Send <- msgCommand{ "GetNickList", "" }

            case "MyINFO":
                if h.state != "preinitialized" && h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }

                // skip first MyINFO since own infos are sent twice
                if h.myInfoReceived == false {
                    h.myInfoReceived = true

                } else {
                    info := reCmdInfo.FindStringSubmatch(msg.Args)
                    if info == nil {
                        return fmt.Errorf("unable to parse info line: %s", msg.Args)
                    }

                    p := &Peer{
                        Nick: info[1],
                        Description: info[2],
                        protocol: info[3],
                        Status: []byte(info[4])[0],
                        Email: info[5],
                        ShareSize: atoui64(info[6]),
                    }

                    if _,exist := h.peers[p.Nick]; !exist {
                        dolog(LevelInfo, "[peer on] %s (%v)", p.Nick, p.ShareSize)
                        if h.client.OnPeerConnected != nil {
                            h.client.OnPeerConnected(p)
                        }

                    } else {
                        if h.client.OnPeerUpdated != nil {
                            h.client.OnPeerUpdated(p)
                        }
                    }

                    h.peers[p.Nick] = p
                }

            case "UserIP":
                if h.state != "preinitialized" && h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }

                // we do not use UserIp to get our own ip, but only to get other
                // ips of other peers
                ips := strings.Split(strings.TrimSuffix(msg.Args, "$$"), "$$")
                for _,ipstr := range ips {
                    args := reCmdUserIP.FindStringSubmatch(ipstr)
                    if args == nil {
                        return fmt.Errorf("unable to parse UserIP args: %s", msg.Args)
                    }
                    // update peer
                    if p,ok := h.peers[args[1]]; ok {
                        p.Ip = args[2]
                        if h.client.OnPeerUpdated != nil {
                            h.client.OnPeerUpdated(p)
                        }
                    }
                }

            case "OpList":
                if h.state != "preinitialized" && h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }

                // reset operators
                for _,p := range h.peers {
                    if p.IsOperator == true {
                        p.IsOperator = false
                        if h.client.OnPeerUpdated != nil {
                            h.client.OnPeerUpdated(p)
                        }
                    }
                }

                // import new operators
                for _,op := range strings.Split(strings.TrimSuffix(msg.Args, "$$"), "$$") {
                    if p,ok := h.peers[op]; ok {
                        p.IsOperator = true
                        if h.client.OnPeerUpdated != nil {
                            h.client.OnPeerUpdated(p)
                        }
                    }
                }

                // switch to initialized
                if h.state != "initialized" {
                    h.state = "initialized"
                    dolog(LevelInfo, "[initialized] %d peers", len(h.peers))
                    if h.client.OnHubConnected != nil {
                        h.client.OnHubConnected()
                    }
                }

            case "UserCommand":
                if h.state != "preinitialized" && h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                args := reCmdUserCommand.FindStringSubmatch(msg.Args)
                if args == nil {
                    return fmt.Errorf("unable to parse UserCommand args: %s", msg.Args)
                }

            case "BotList":
                if h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }

                // reset bots
                for _,p := range h.peers {
                    if p.IsBot == true {
                        p.IsBot = false
                        if h.client.OnPeerUpdated != nil {
                            h.client.OnPeerUpdated(p)
                        }
                    }
                }

                // import new bots
                bots := strings.Split(strings.TrimSuffix(msg.Args, "$$"), "$$")
                for _,bot := range bots {
                    if p,ok := h.peers[bot]; ok {
                        p.IsBot = true
                        if h.client.OnPeerUpdated != nil {
                            h.client.OnPeerUpdated(p)
                        }
                    }
                }

            case "Quit":
                if h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                nick := msg.Args
                if _,ok := h.peers[nick]; ok {
                    p := h.peers[nick]
                    delete(h.peers, nick)
                    dolog(LevelInfo, "[peer off] %s", p.Nick)
                    if h.client.OnPeerDisconnected != nil {
                        h.client.OnPeerDisconnected(p)
                    }

                } else {
                    return fmt.Errorf("received quit() on unconnected peer: %s", nick)
                }

            case "ForceMove":
                // means disconnect and reconnect to provided address
                // we just disconnect
                return fmt.Errorf("received force move")

            case "Search":
                // searches can be received even before initialization; ignore them
                if h.state == "initialized" {
                    req,err := newSearchRequest(msg.Args)
                    if err != nil {
                        dolog(LevelDebug, "invalid search request (%s): %s", msg.Args, err)
                    } else {
                        h.client.onSearchRequest(req)
                    }
                }

            case "SR":
                if h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                res,err := newSearchResult(false, msg.Args)
                if err != nil {
                    return fmt.Errorf("unable to parse search result")
                }
                dolog(LevelInfo, "[search res] %+v", res)

                if h.client.OnSearchResult != nil {
                    h.client.OnSearchResult(res)
                }

            case "ConnectToMe":
                if h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                args := reCmdConnectToMe.FindStringSubmatch(msg.Args)
                if args == nil {
                    return fmt.Errorf("unable to parse ConnectToMe args: %s", msg.Args)
                }
                ip := args[2]
                port := atoui(args[3])
                tls := (args[4] != "")
                if tls == true && h.client.conf.PeerEncryptionMode == DisableEncryption {
                    dolog(LevelInfo, "received encrypted connect to me request but encryption is disabled, skipping")
                } else if tls == false && h.client.conf.PeerEncryptionMode == ForceEncryption {
                    dolog(LevelInfo, "received plain connect to me request but encryption is forced, skipping")
                } else {
                    newPeerConn(h.client, tls, false, nil, ip, port)
                }

            case "RevConnectToMe":
                if h.state != "initialized" {
                    return fmt.Errorf("[%s] invalid state: %s", msg.Key, h.state)
                }
                args := reCmdRevConnectToMe.FindStringSubmatch(msg.Args)
                if args == nil {
                    return fmt.Errorf("unable to parse RevConnectToMe args: %s", msg.Args)
                }
                // we can process RevConnectToMe only in active mode
                if h.client.conf.ModePassive == false {
                    h.client.connectToMe(args[1])
                }

            default:
                return fmt.Errorf("unhandled: [%s] %s", msg.Key, msg.Args)
        }

    case msgPublicChat:
        if h.client.OnPublicMessage != nil {
            p := h.peers[msg.Author]
            if p == nil { // create a dummy peer if not found
                p = &Peer{ Nick: msg.Author }
            }
            h.client.OnPublicMessage(p, msg.Content)
        }

    case msgPrivateChat:
        if h.client.OnPrivateMessage != nil {
            p := h.peers[msg.Author]
            if p == nil { // create a dummy peer if not found
                p = &Peer{ Nick: msg.Author }
            }
            h.client.OnPrivateMessage(p, msg.Content)
        }
    }
    return nil
}
