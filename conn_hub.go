package dctoolkit

import (
    "fmt"
    "net"
    "time"
    "strings"
    "crypto/tls"
)

type connHub struct {
    client          *Client
    state           string
    conn            protocol
    uniqueCmds      map[string]struct{}
    myInfoReceived  bool
}

func newConnHub(client *Client) error {
    client.connHub = &connHub{
        client: client,
        state: "uninitialized",
        uniqueCmds: make(map[string]struct{}),
    }
    return nil
}

// HubConnect must be called only when HubManualConnect is turned on. It starts
// the connection to the hub.
func (c *Client) HubConnect() {
    if c.connHub.state != "uninitialized" {
        return
    }

    c.connHub.state = "connecting"
    c.wg.Add(1)
    go c.connHub.do()
}

func (h *connHub) terminate() {
    switch h.state {
    case "terminated":
        return

    case "initialized":
        h.conn.Terminate()

    default:
        panic(fmt.Errorf("Terminate() unsupported in state '%s'", h.state))
    }
    h.state = "terminated"
}

func (h *connHub) do() {
    defer h.client.wg.Done()

    err := func() error {
        var rawconn net.Conn
        var err error
        for i := uint(0); i < h.client.conf.HubConnTries; i++ {
            if i > 0 {
                dolog(LevelInfo, "retrying... (%s)", err)
            }

            var ips []net.IP
            ips,err = net.LookupIP(h.client.hubHostname)
            if err != nil {
                break
            }

            h.client.hubSolvedIp = ips[0].String()
            rawconn,err = net.DialTimeout("tcp",
                fmt.Sprintf("%s:%d", h.client.hubSolvedIp, h.client.hubPort), 10 * time.Second)
            if err == nil {
                break
            }
        }
        if err != nil {
            return err
        }

        // activate TCP keepalive
        if err := rawconn.(*net.TCPConn).SetKeepAlive(true); err != nil {
            return err
        }
        if err := rawconn.(*net.TCPConn).SetKeepAlivePeriod(60 * time.Second); err != nil {
            return err
        }

        if h.client.hubIsEncrypted == true {
            rawconn = tls.Client(rawconn, &tls.Config{ InsecureSkipVerify: true })
        }

        // do not use read timeout since hub does not send data continuously
        if h.client.hubIsAdc == true {
            h.conn = newProtocolAdc("h", rawconn, false, true)
        } else {
            h.conn = newProtocolNmdc("h", rawconn, false, true)
        }

        exit := false
        h.client.Safe(func() {
            if h.state == "terminated" {
                h.conn.Terminate()
                exit = true
                return
            }
            dolog(LevelInfo, "[hub connected] %s", connRemoteAddr(rawconn))
            h.state = "connected"
        })
        if exit == true {
            return errorTerminated
        }

        if h.client.hubIsAdc == true {
            h.conn.Write(&msgAdcHSupports{
                msgAdcTypeH{},
                msgAdcKeySupports{
                    Features: []string{ "ADBAS0", "ADBASE", "ADTIGR", "ADUCM0", "ADBLO0", "ADZLIF"},
                },
            })
        }

        for {
            msg,err := h.conn.Read()
            if err != nil {
                h.conn.Terminate()
                return err
            }

            err = h.handleMessage(msg)
            if err != nil {
                h.conn.Terminate()
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

func (h *connHub) handleMessage(rawmsg msgDecodable) error {
    h.client.mutex.Lock()
    defer h.client.mutex.Unlock()

    if h.state == "terminated" {
        return errorTerminated
    }

    switch msg := rawmsg.(type) {
    case *msgAdcISupports:
        if h.state != "connected" {
            return fmt.Errorf("[Supports] invalid state: %s", h.state)
        }
        h.state = "supports"

    case *msgAdcISessionId:
        if h.state != "supports" {
            return fmt.Errorf("[SessionId] invalid state: %s", h.state)
        }
        h.state = "sessionid"
        h.client.sessionId = msg.Sid

    case *msgAdcIInfos:
        if h.state != "sessionid" {
            return fmt.Errorf("[Infos] invalid state: %s", h.state)
        }
        h.state = "hubinfos"

        for key,desc := range map[string]string{
            adcInfoName: "name",
            adcInfoSoftware: "software",
            adcInfoVersion: "version",
            adcInfoDescription: "description",
        } {
            if val,ok := msg.Fields[key]; ok {
                dolog(LevelInfo, "[hub info] [%s] %s", desc, val)
            }
        }

        supports := []string{ "SEGA" }
        //if h.client.PeerEncryptionMode != DisableEncryption {
        //    supports = append(supports, "ADCS")
        //}
        if h.client.conf.ModePassive == false {
            supports = append(supports, "TCP4", "UDP4")
        }

        fields := map[string]string{
            adcInfoName: h.client.conf.Nick,
            adcInfoDescription: h.client.conf.Description,
            adcInfoShareCount: fmt.Sprintf("%d", h.client.shareCount),
            adcInfoShareSize: fmt.Sprintf("%d", h.client.shareSize),
            adcInfoHubUnregisteredCount: fmt.Sprintf("%d", h.client.conf.HubUnregisteredCount),
            adcInfoHubRegisteredCount: fmt.Sprintf("%d", h.client.conf.HubRegisteredCount),
            adcInfoHubOperatorCount: fmt.Sprintf("%d", h.client.conf.HubOperatorCount),
            adcInfoClientId: adcBase32Encode(h.client.clientId),
            adcInfoPrivateId: adcBase32Encode(h.client.privateId),
            adcInfoVersion: h.client.conf.ListGenerator,
            adcInfoSupports: strings.Join(supports, ","),
        }

        if h.client.conf.ModePassive == false {
            fields[adcInfoIp] = h.client.ip
            fields[adcInfoUdpPort] = fmt.Sprintf("%d", h.client.conf.UdpPort)
        }

        h.conn.Write(&msgAdcBInfos{
            msgAdcTypeB{ SessionId: h.client.sessionId },
            msgAdcKeyInfos{ Fields: fields },
        })

    case *msgAdcIGetPass:
        if h.state != "hubinfos" {
            return fmt.Errorf("[Sup] invalid state: %s", h.state)
        }
        h.state = "getpass"

        hasher := tigerNew()
        hasher.Write([]byte(h.client.conf.Password))
        hasher.Write(msg.Data)
        data := hasher.Sum(nil)

        h.conn.Write(&msgAdcHPass{
            msgAdcTypeH{},
            msgAdcKeyPass{ Data: data },
        })

    case *msgAdcIStatus:
        if msg.Code != 0 {
            return fmt.Errorf("error (%d): %s", msg.Code, msg.Message)
        }

    case *msgAdcBInfos:
        p,exist := h.client.peers[msg.Fields[adcInfoName]]
        if exist == false {
            p = &Peer{ Nick: msg.Fields[adcInfoName] }
        }

        p.AdcSessionId = msg.SessionId

        for key,val := range msg.Fields {
            switch key {
            case adcInfoDescription: p.Description = val
            case adcInfoEmail: p.Email = val
            case adcInfoShareSize: p.ShareSize = atoui64(val)
            case adcInfoIp: p.Ip = val
            case adcInfoClientId: p.AdcClientId = adcBase32Decode(val)
            case adcInfoSupports: p.AdcSupports = strings.Split(val, ",")
            case adcInfoCategory:
                ct := atoui(val)
                p.IsBot = (ct & 1) != 0
                p.IsOperator = ((ct & 4) | (ct & 8) | (ct & 16)) != 0
            }
        }

        h.client.peers[p.Nick] = p

        if exist == false {
            dolog(LevelInfo, "[peer on] %s (%v)", p.Nick, p.ShareSize)
            if h.client.OnPeerConnected != nil {
                h.client.OnPeerConnected(p)
            }

        } else {
            if h.client.OnPeerUpdated != nil {
                h.client.OnPeerUpdated(p)
            }
        }

    case *msgAdcIQuit:
        // self quit, used instead of ForceMove
        if msg.SessionId == h.client.sessionId {
            return fmt.Errorf("received Quit message: %s", msg.Reason)

        // peer quit
        } else {
            // solve session id
            p := func() *Peer {
                for _,p := range h.client.peers {
                    if p.AdcSessionId == msg.SessionId {
                        return p
                    }
                }
                return nil
            }()
            if p != nil {
                delete(h.client.peers, p.Nick)
                dolog(LevelInfo, "[peer off] %s", p.Nick)
                if h.client.OnPeerDisconnected != nil {
                    h.client.OnPeerDisconnected(p)
                }
            }
        }

    case *msgAdcICommand:
        // switch to initialized
        if h.state != "initialized" {
            h.state = "initialized"
            dolog(LevelInfo, "[initialized] %d peers", len(h.client.peers))
            if h.client.OnHubConnected != nil {
                h.client.OnHubConnected()
            }
        }

    case *msgAdcBMessage:
        // solve session id
        p := func() *Peer {
            for _,p := range h.client.peers {
                if p.AdcSessionId == msg.SessionId {
                    return p
                }
            }
            return nil
        }()
        if p == nil {
            return fmt.Errorf("private message with unknown author")
        }
        dolog(LevelInfo, "[PUB] <%s> %s", p.Nick, msg.Content)
        if h.client.OnMessagePublic != nil {
            h.client.OnMessagePublic(p, msg.Content)
        }

    case *msgAdcDMessage:
        // solve session id
        p := func() *Peer {
            for _,p := range h.client.peers {
                if p.AdcSessionId == msg.AuthorId {
                    return p
                }
            }
            return nil
        }()
        if p == nil {
            return fmt.Errorf("private message with unknown author")
        }
        dolog(LevelInfo, "[PRIV] <%s> %s", p.Nick, msg.Content)
        if h.client.OnMessagePrivate != nil {
            h.client.OnMessagePrivate(p, msg.Content)
        }

    case *msgAdcBSearchRequest:
        // TODO
        /*temp := &msgNmdcSearchRequest{}
        if val,ok := msg.Fields["LE"]; ok {
            temp.MaxSize = atoui(val)
        }
        if val,ok := msg.Fields["GE"]; ok {
            temp.MinSize = atoui(val)
        }
        if val,ok := msg.Fields["TY"]; ok {
            if val == "1" {
                temp.Type = TypeAny
            } else if val == "2" {
                temp.Type = TypeFolder
            }
        }
        if val,ok := msg.Fields["AN"]; ok {
            temp.Query = val
        }
        h.client.onSearchRequest(temp)*/

    case *msgNmdcLock:
        if h.state != "connected" {
            return fmt.Errorf("[Lock] invalid state: %s", h.state)
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

        h.conn.Write(&msgNmdcSupports{ Features: hubSupports })
        h.conn.Write(&msgNmdcKey{ Key: nmdcComputeKey([]byte(msg.Values[0])) })
        h.conn.Write(&msgNmdcValidateNick{ Nick: h.client.conf.Nick })

    case *msgNmdcSupports:
        if h.state != "lock" {
            return fmt.Errorf("[Supports] invalid state: %s", h.state)
        }
        h.state = "preinitialized"

    // flexhub send HubName just after lock
    // HubName can also be sent twice
    case *msgNmdcHubName:
        if h.state != "preinitialized" && h.state != "lock" {
            return fmt.Errorf("[HubName] invalid state: %s", h.state)
        }

    case *msgNmdcZon:
        if h.state != "initialized" && h.state != "preinitialized" {
            return fmt.Errorf("[ZOn] invalid state: %s", h.state)
        }
        if h.client.conf.HubDisableCompression == true {
            return fmt.Errorf("zlib requested but zlib is disabled")
        }
        if err := h.conn.SetReadCompressionOn(); err != nil {
            return err
        }

    case *msgNmdcHubTopic:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[HubTopic] invalid state: %s", h.state)
        }
        if _,ok := h.uniqueCmds["HubTopic"]; ok {
            return fmt.Errorf("HubTopic sent twice")
        }
        h.uniqueCmds["HubTopic"] = struct{}{}

    case *msgNmdcGetPass:
        if h.state != "preinitialized" {
            return fmt.Errorf("[GetPass] invalid state: %s", h.state)
        }
        h.conn.Write(&msgNmdcMyPass{ Pass: h.client.conf.Password })
        if _,ok := h.uniqueCmds["GetPass"]; ok {
            return fmt.Errorf("GetPass sent twice")
        }
        h.uniqueCmds["GetPass"] = struct{}{}

    case *msgNmdcLoggedIn:
        if h.state != "preinitialized" {
            return fmt.Errorf("[LoggedIn] invalid state: %s", h.state)
        }
        if _,ok := h.uniqueCmds["LoggedIn"]; ok {
            return fmt.Errorf("LoggedIn sent twice")
        }
        h.uniqueCmds["LoggedIn"] = struct{}{}

    case *msgNmdcHello:
        if h.state != "preinitialized" {
            return fmt.Errorf("[Hello] invalid state: %s", h.state)
        }
        if _,ok := h.uniqueCmds["Hello"]; ok {
            return fmt.Errorf("Hello sent twice")
        }
        h.uniqueCmds["Hello"] = struct{}{}

        // The last version of the Neo-Modus client was 1.0091 and is what is commonly used by current clients
        // https://github.com/eiskaltdcpp/eiskaltdcpp/blob/1e72256ac5e8fe6735f81bfbc3f9d90514ada578/dcpp/NmdcHub.h#L119
        h.conn.Write(&msgNmdcVersion{})
        h.client.myInfo()
        h.conn.Write(&msgNmdcGetNickList{})

    case *msgNmdcMyInfo:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[MyInfo] invalid state: %s", h.state)
        }

        // skip first MyINFO since own infos are sent twice
        if h.myInfoReceived == false {
            h.myInfoReceived = true

        } else {
            p,exist := h.client.peers[msg.Nick]
            if exist == false {
                p = &Peer{ Nick: msg.Nick }
            }

            p.Description = msg.Description
            p.Email = msg.Email
            p.ShareSize = msg.ShareSize
            p.NmdcConnection = msg.Connection
            p.NmdcStatusByte = msg.StatusByte

            h.client.peers[p.Nick] = p

            if exist == false {
                dolog(LevelInfo, "[peer on] %s (%v)", p.Nick, p.ShareSize)
                if h.client.OnPeerConnected != nil {
                    h.client.OnPeerConnected(p)
                }

            } else {
                if h.client.OnPeerUpdated != nil {
                    h.client.OnPeerUpdated(p)
                }
            }
        }

    case *msgNmdcUserIp:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[UserIp] invalid state: %s", h.state)
        }

        // we do not use UserIp to get our own ip, but only to get other
        // ips of other peers
        for peer,ip := range msg.Ips {
            // update peer
            if p,ok := h.client.peers[peer]; ok {
                p.Ip = ip
                if h.client.OnPeerUpdated != nil {
                    h.client.OnPeerUpdated(p)
                }
            }
        }

    case *msgNmdcOpList:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[OpList] invalid state: %s", h.state)
        }

        // reset operators
        for _,p := range h.client.peers {
            if p.IsOperator == true {
                p.IsOperator = false
                if h.client.OnPeerUpdated != nil {
                    h.client.OnPeerUpdated(p)
                }
            }
        }

        // import new operators
        for _,op := range msg.Ops {
            if p,ok := h.client.peers[op]; ok {
                p.IsOperator = true
                if h.client.OnPeerUpdated != nil {
                    h.client.OnPeerUpdated(p)
                }
            }
        }

        // switch to initialized
        if h.state != "initialized" {
            h.state = "initialized"
            dolog(LevelInfo, "[initialized] %d peers", len(h.client.peers))
            if h.client.OnHubConnected != nil {
                h.client.OnHubConnected()
            }
        }

    case *msgNmdcUserCommand:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[UserCommand] invalid state: %s", h.state)
        }

    case *msgNmdcBotList:
        if h.state != "initialized" {
            return fmt.Errorf("[BotList] invalid state: %s", h.state)
        }

        // reset bots
        for _,p := range h.client.peers {
            if p.IsBot == true {
                p.IsBot = false
                if h.client.OnPeerUpdated != nil {
                    h.client.OnPeerUpdated(p)
                }
            }
        }

        // import new bots
        for _,bot := range msg.Bots {
            if p,ok := h.client.peers[bot]; ok {
                p.IsBot = true
                if h.client.OnPeerUpdated != nil {
                    h.client.OnPeerUpdated(p)
                }
            }
        }

    case *msgNmdcQuit:
        if h.state != "initialized" {
            return fmt.Errorf("[Quit] invalid state: %s", h.state)
        }
        p,ok := h.client.peers[msg.Nick]
        if ok {
            delete(h.client.peers, p.Nick)
            dolog(LevelInfo, "[peer off] %s", p.Nick)
            if h.client.OnPeerDisconnected != nil {
                h.client.OnPeerDisconnected(p)
            }
        }

    case *msgNmdcForceMove:
        // means disconnect and reconnect to provided address
        // we just disconnect
        return fmt.Errorf("received force move")

    case *msgNmdcSearchRequest:
        // searches can be received even before initialization; ignore them
        if h.state == "initialized" {
            h.client.onSearchRequest(msg)
        }

    case *msgNmdcSearchResult:
        if h.state != "initialized" {
            return fmt.Errorf("[SearchResult] invalid state: %s", h.state)
        }
        sr := &SearchResult{
            IsActive: false,
            Nick: msg.Nick,
            Path: msg.Path,
            SlotAvail: msg.SlotAvail,
            SlotCount: msg.SlotCount,
            TTH: msg.TTH,
            IsDir: msg.IsDir,
            HubIp: msg.HubIp,
            HubPort: msg.HubPort,
        }
        dolog(LevelInfo, "[search res] %+v", sr)

        if h.client.OnSearchResult != nil {
            h.client.OnSearchResult(sr)
        }

    case *msgNmdcConnectToMe:
        if h.state != "initialized" && h.state != "preinitialized" {
            return fmt.Errorf("[ConnectToMe] invalid state: %s", h.state)
        }
        if msg.Encrypted == true && h.client.conf.PeerEncryptionMode == DisableEncryption {
            dolog(LevelInfo, "received encrypted connect to me request but encryption is disabled, skipping")
        } else if msg.Encrypted == false && h.client.conf.PeerEncryptionMode == ForceEncryption {
            dolog(LevelInfo, "received plain connect to me request but encryption is forced, skipping")
        } else {
            newConnPeer(h.client, msg.Encrypted, false, nil, msg.Ip, msg.Port)
        }

    case *msgNmdcRevConnectToMe:
        if h.state != "initialized" && h.state != "preinitialized" {
            return fmt.Errorf("[RevConnectToMe] invalid state: %s", h.state)
        }
        // we can process RevConnectToMe only in active mode
        if h.client.conf.ModePassive == false {
            h.client.connectToMe(msg.Author)
        }

    case *msgNmdcPublicChat:
        p := h.client.peers[msg.Author]
        if p == nil { // create a dummy peer if not found
            p = &Peer{ Nick: msg.Author }
        }
        dolog(LevelInfo, "[PUB] <%s> %s", p.Nick, msg.Content)
        if h.client.OnMessagePublic != nil {
            h.client.OnMessagePublic(p, msg.Content)
        }

    case *msgNmdcPrivateChat:
        p := h.client.peers[msg.Author]
        if p == nil { // create a dummy peer if not found
            p = &Peer{ Nick: msg.Author }
        }
        dolog(LevelInfo, "[PRIV] <%s> %s", p.Nick, msg.Content)
        if h.client.OnMessagePrivate != nil {
            h.client.OnMessagePrivate(p, msg.Content)
        }

    default:
        return fmt.Errorf("unhandled: %T %+v", rawmsg, rawmsg)
    }
    return nil
}
