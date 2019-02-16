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
        if h.client.protoIsAdc == true {
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

        if h.client.protoIsAdc == true {
            h.conn.Write(&msgAdcHSupports{
                msgAdcTypeH{},
                msgAdcKeySupports{ map[string]struct{}{
                     "ADBAS0": struct{}{},
                     "ADBASE": struct{}{},
                     "ADTIGR": struct{}{},
                     "ADUCM0": struct{}{},
                     "ADBLO0": struct{}{},
                     "ADZLIF": struct{}{},
                } },
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
    case *msgAdcIStatus:
        if msg.Code != 0 {
            return fmt.Errorf("error (%d): %s", msg.Code, msg.Message)
        }

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
            adcFieldName: "name",
            adcFieldSoftware: "software",
            adcFieldVersion: "version",
            adcFieldDescription: "description",
        } {
            if val,ok := msg.Fields[key]; ok {
                dolog(LevelInfo, "[hub info] [%s] %s", desc, val)
            }
        }

        h.client.sendInfos(true)

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

    case *msgAdcBInfos:
        exists := true
        p := h.client.peerBySessionId(msg.SessionId)
        if p == nil {
            exists = false

            // adcFieldName is mandatory for peer creation
            if _,ok := msg.Fields[adcFieldName]; !ok {
                return fmt.Errorf("adcFieldName not sent")
            }
            if h.client.peerByNick(msg.Fields[adcFieldName]) != nil {
                return fmt.Errorf("trying to create already-existent peer")
            }

            p = &Peer{
                Nick: msg.Fields[adcFieldName],
                adcSessionId: msg.SessionId,
            }
        }

        for key,val := range msg.Fields {
            switch key {
            case adcFieldDescription: p.Description = val
            case adcFieldEmail: p.Email = val
            case adcFieldShareSize: p.ShareSize = atoui64(val)
            case adcFieldIp: p.Ip = val
            case adcFieldUdpPort: p.adcUdpPort = atoui(val)
            case adcFieldClientId: p.adcClientId = dcBase32Decode(val)
            case adcFieldSupports:
                p.adcSupports = make(map[string]struct{})
                for _,feat := range strings.Split(val, ",") {
                    p.adcSupports[feat] = struct{}{}
                }

            case adcFieldCategory:
                ct := atoui(val)
                p.IsBot = (ct & 1) != 0
                p.IsOperator = ((ct & 4) | (ct & 8) | (ct & 16)) != 0

            case adcFieldSoftware:
                p.Client = val
            case adcFieldVersion:
                p.Version = val
            }
        }

        // a peer is active if it supports udp4, it exposes udp port and ip
        p.IsPassive = true
        if _,ok := p.adcSupports["UDP4"]; ok {
            if p.Ip != "" && p.adcUdpPort != 0 {
                p.IsPassive = false
            }
        }

        if exists == false {
            h.client.handlePeerConnected(p)
        } else {
            h.client.handlePeerUpdated(p)
        }

    case *msgAdcIQuit:
        // self quit, used instead of ForceMove
        if msg.SessionId == h.client.sessionId {
            return fmt.Errorf("received Quit message: %s", msg.Reason)

        // peer quit
        } else {
            p := h.client.peerBySessionId(msg.SessionId)
            if p != nil {
                h.client.handlePeerDisconnected(p)
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
        p := h.client.peerBySessionId(msg.SessionId)
        if p == nil {
            return fmt.Errorf("private message with unknown author")
        }
        h.client.handlePublicMessage(p, msg.Content)

    case *msgAdcDMessage:
        p := h.client.peerBySessionId(msg.AuthorId)
        if p == nil {
            return fmt.Errorf("private message with unknown author")
        }
        h.client.handlePrivateMessage(p, msg.Content)

    case *msgAdcBSearchRequest:
        h.client.handleAdcSearchRequest(msg.SessionId, &msg.msgAdcKeySearchRequest)

    case *msgAdcFSearchRequest:
        if _,ok := msg.RequiredFeatures["TCP4"]; ok {
            if h.client.conf.IsPassive == true {
                dolog(LevelDebug, "[F warning] we are in passive and author requires active")
                return nil
            }
        }
        h.client.handleAdcSearchRequest(msg.SessionId, &msg.msgAdcKeySearchRequest)

    case *msgAdcDSearchResult:
        p := h.client.peerBySessionId(msg.AuthorId)
        if p == nil {
            return fmt.Errorf("search result with unknown author")
        }
        h.client.handleSearchResult(adcMsgToSearchResult(false, p, &msg.msgAdcKeySearchResult))

    case *msgAdcDConnectToMe:
        p := h.client.peerBySessionId(msg.AuthorId)
        if p == nil {
            return fmt.Errorf("connecttome with unknown author")
        }
        newConnPeer(h.client, false, false, nil, p.Ip, msg.TcpPort, msg.Token)

    case *msgAdcDRevConnectToMe:
        p := h.client.peerBySessionId(msg.AuthorId)
        if p == nil {
            return fmt.Errorf("revconnecttome with unknown author")
        }
        h.client.handlePeerRevConnectToMe(p)

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
        // this must be provided, otherwise the final S is stripped from ConnectToMe
        if h.client.conf.PeerEncryptionMode != DisableEncryption {
            hubSupports = append(hubSupports, "TLS")
        }

        h.conn.Write(&msgNmdcSupports{ Features: hubSupports })
        h.conn.Write(&msgNmdcKey{ Key: nmdcComputeKey([]byte(msg.Values[0])) })
        h.conn.Write(&msgNmdcValidateNick{ Nick: h.client.conf.Nick })

    case *msgNmdcValidateDenide:
        return fmt.Errorf("forbidden nickname")

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

    case *msgNmdcBadPassword:
        return fmt.Errorf("wrong password")

    case *msgNmdcHubIsFull:
        return fmt.Errorf("hub is full")

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
        h.client.sendInfos(true)
        h.conn.Write(&msgNmdcGetNickList{})

    case *msgNmdcMyInfo:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[MyInfo] invalid state: %s", h.state)
        }
        exists := true
        p := h.client.peerByNick(msg.Nick)
        if p == nil {
            exists = false
            p = &Peer{ Nick: msg.Nick }
        }

        p.Description = msg.Description
        p.Email = msg.Email
        p.ShareSize = msg.ShareSize
        p.nmdcConnection = msg.Connection
        p.nmdcStatusByte = msg.StatusByte
        if msg.Mode != "" { // set mode only if it has been sent
            p.IsPassive = (msg.Mode == "P")
        }
        if msg.Client != "" {
            p.Client = msg.Client
        }
        if msg.Version != "" {
            p.Version = msg.Version
        }

        if exists == false {
            h.client.handlePeerConnected(p)
        } else {
            h.client.handlePeerUpdated(p)
        }

    case *msgNmdcUserIp:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[UserIp] invalid state: %s", h.state)
        }

        // we do not use UserIp to get our own ip, but only to get other
        // ips of other peers
        for peer,ip := range msg.Ips {
            // update peer
            p := h.client.peerByNick(peer)
            if p != nil {
                p.Ip = ip
                h.client.handlePeerUpdated(p)
            }
        }

    case *msgNmdcOpList:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[OpList] invalid state: %s", h.state)
        }

        for _,p := range h.client.peers {
            _,isOp := msg.Ops[p.Nick]
            if isOp != p.IsOperator {
                p.IsOperator = isOp
                h.client.handlePeerUpdated(p)
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

    case *msgNmdcBotList:
        if h.state != "initialized" {
            return fmt.Errorf("[BotList] invalid state: %s", h.state)
        }

        for _,p := range h.client.peers {
            _,isBot := msg.Bots[p.Nick]
            if isBot != p.IsBot {
                p.IsBot = isBot
                h.client.handlePeerUpdated(p)
            }
        }

    case *msgNmdcUserCommand:
        if h.state != "preinitialized" && h.state != "initialized" {
            return fmt.Errorf("[UserCommand] invalid state: %s", h.state)
        }

    case *msgNmdcQuit:
        if h.state != "initialized" {
            return fmt.Errorf("[Quit] invalid state: %s", h.state)
        }
        p := h.client.peerByNick(msg.Nick)
        if p != nil {
            h.client.handlePeerDisconnected(p)
        }

    case *msgNmdcForceMove:
        // means disconnect and reconnect to provided address
        // we just disconnect
        return fmt.Errorf("received force move")

    case *msgNmdcSearchRequest:
        // searches can be received even before initialization; ignore them
        if h.state == "initialized" {
            h.client.handleNmdcSearchRequest(msg)
        }

    case *msgNmdcSearchResult:
        if h.state != "initialized" {
            return fmt.Errorf("[SearchResult] invalid state: %s", h.state)
        }
        p := h.client.peerByNick(msg.Nick)
        if p != nil {
            h.client.handleSearchResult(nmdcMsgToSearchResult(false, p, msg))
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
            newConnPeer(h.client, msg.Encrypted, false, nil, msg.Ip, msg.Port, "")
        }

    case *msgNmdcRevConnectToMe:
        if h.state != "initialized" && h.state != "preinitialized" {
            return fmt.Errorf("[RevConnectToMe] invalid state: %s", h.state)
        }
        p := h.client.peerByNick(msg.Author)
        if p != nil {
            h.client.handlePeerRevConnectToMe(p)
        }

    case *msgNmdcPublicChat:
        p := h.client.peerByNick(msg.Author)
        if p == nil { // create a dummy peer if not found
            p = &Peer{ Nick: msg.Author }
        }
        h.client.handlePublicMessage(p, msg.Content)

    case *msgNmdcPrivateChat:
        p := h.client.peerByNick(msg.Author)
        if p == nil { // create a dummy peer if not found
            p = &Peer{ Nick: msg.Author }
        }
        h.client.handlePrivateMessage(p, msg.Content)

    default:
        return fmt.Errorf("unhandled: %T %+v", rawmsg, rawmsg)
    }
    return nil
}
