package dctoolkit

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"
)

// HubField is a name of a hub information field.
type HubField string

const (
	HubName        = HubField("name")
	HubTopic       = HubField("topic")
	HubSoftware    = HubField("software")
	HubVersion     = HubField("version")
	HubDescription = HubField("description")
)

type connHub struct {
	client             *Client
	terminateRequested bool
	terminate          chan struct{}
	state              string
	conn               protocol
	passwordSent       bool
	uniqueCmds         map[string]struct{}
}

func newConnHub(client *Client) error {
	client.connHub = &connHub{
		client:     client,
		terminate:  make(chan struct{}, 1),
		state:      "disconnected",
		uniqueCmds: make(map[string]struct{}),
	}
	return nil
}

// HubConnect starts the connection to the hub. It must be called only when
// HubManualConnect is true.
func (c *Client) HubConnect() {
	if c.connHub.state != "disconnected" {
		return
	}
	c.connHub.state = "connecting"
	c.wg.Add(1)
	go c.connHub.do()
}

func (h *connHub) close() {
	if h.terminateRequested == true {
		return
	}
	h.terminateRequested = true
	h.terminate <- struct{}{}
}

func (h *connHub) do() {
	defer h.client.wg.Done()

	err := func() error {
		// resolve hub ip
		ips, err := net.LookupIP(h.client.hubHostname)
		if err != nil {
			return err
		}
		h.client.hubSolvedIp = ips[0].String()

		// connect to hub
		ce := newConnEstablisher(
			fmt.Sprintf("%s:%d", h.client.hubSolvedIp, h.client.hubPort),
			10*time.Second, h.client.conf.HubConnTries)

		select {
		case <-h.terminate:
			return errorTerminated
		case <-ce.Wait:
		}

		if ce.Error != nil {
			return ce.Error
		}

		// hub connected
		rawconn := ce.Conn
		if h.client.hubIsEncrypted == true {
			rawconn = tls.Client(rawconn, &tls.Config{InsecureSkipVerify: true})
		}

		// do not use read timeout since hub does not send data continuously
		if h.client.protoIsAdc == true {
			h.conn = newProtocolAdc("h", rawconn, false, true)
		} else {
			h.conn = newProtocolNmdc("h", rawconn, false, true)
		}

		if h.client.conf.HubDisableKeepAlive == false {
			keepaliver := newHubKeepAliver(h)
			defer keepaliver.Close()
		}

		dolog(LevelInfo, "[hub] connected (%s)", rawconn.RemoteAddr())

		if h.client.protoIsAdc == true {
			features := map[string]struct{}{
				adcFeatureBas0:         {},
				adcFeatureBase:         {},
				adcFeatureTiger:        {},
				adcFeatureUserCommands: {},
			}
			if h.client.conf.HubDisableCompression == false {
				features[adcFeatureZlibFull] = struct{}{}
			}
			h.conn.Write(&msgAdcHSupports{
				msgAdcTypeH{},
				msgAdcKeySupports{features},
			})
		}

		h.client.Safe(func() {
			h.state = "connected"
		})

		readDone := make(chan error)
		go func() {
			readDone <- func() error {
				for {
					msg, err := h.conn.Read()
					if err != nil {
						return err
					}

					h.client.Safe(func() {
						err = h.handleMessage(msg)
					})
					if err != nil {
						return err
					}
				}
			}()
		}()

		select {
		case <-h.terminate:
			h.conn.Close()
			<-readDone
			return errorTerminated

		case err := <-readDone:
			h.conn.Close()
			return err
		}
	}()

	h.client.Safe(func() {
		if h.terminateRequested != true {
			dolog(LevelInfo, "ERR: %s", err)

			if h.client.OnHubError != nil {
				h.client.OnHubError(err)
			}
		}

		dolog(LevelInfo, "[hub] disconnected")

		// close client too
		h.client.Close()
	})
}

func (h *connHub) handleMessage(msgi msgDecodable) error {
	switch msg := msgi.(type) {
	case *msgAdcKeepAlive:

	case *msgAdcIZon:
		if h.client.conf.HubDisableCompression == true {
			return fmt.Errorf("zlib requested but zlib is disabled")
		}
		if err := h.conn.ReaderEnableZlib(); err != nil {
			return err
		}

	case *msgAdcIStatus:
		if msg.Type != adcStatusOk {
			return fmt.Errorf("error: %+v", msg)
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
		h.client.sendInfos(true)

	case *msgAdcIInfos:
		for key, val := range msg.Fields {
			var klabel HubField
			switch key {
			case adcFieldName:
				klabel = HubName
			case adcFieldSoftware:
				klabel = HubSoftware
			case adcFieldVersion:
				klabel = HubVersion
			case adcFieldDescription:
				klabel = HubDescription
			default:
				klabel = HubField(key)
			}
			if h.client.OnHubInfo != nil {
				h.client.OnHubInfo(klabel, val)
			}
			dolog(LevelInfo, "[hub] [%s] %s", klabel, val)
		}

	case *msgAdcIMsg:
		dolog(LevelInfo, "[hub] %s", msg.Content)

	case *msgAdcIGetPass:
		if h.state != "sessionid" {
			return fmt.Errorf("[Sup] invalid state: %s", h.state)
		}
		h.state = "getpass"

		hasher := newTiger()
		hasher.Write([]byte(h.client.conf.Password))
		hasher.Write(msg.Data)
		data := hasher.Sum(nil)

		h.passwordSent = true
		h.conn.Write(&msgAdcHPass{
			msgAdcTypeH{},
			msgAdcKeyPass{Data: data},
		})

	case *msgAdcBInfos:
		exists := true
		p := h.client.peerBySessionId(msg.SessionId)
		if p == nil {
			exists = false

			// adcFieldName is mandatory for peer creation
			if _, ok := msg.Fields[adcFieldName]; !ok {
				return fmt.Errorf("adcFieldName not sent")
			}
			if h.client.peerByNick(msg.Fields[adcFieldName]) != nil {
				return fmt.Errorf("trying to create already-existent peer")
			}

			p = &Peer{
				Nick:         msg.Fields[adcFieldName],
				adcSessionId: msg.SessionId,
			}
		}

		for key, val := range msg.Fields {
			switch key {
			case adcFieldDescription:
				p.Description = val
			case adcFieldEmail:
				p.Email = val
			case adcFieldShareSize:
				p.ShareSize = atoui64(val)
			case adcFieldIp:
				p.Ip = val
			case adcFieldUdpPort:
				p.adcUdpPort = atoui(val)
			case adcFieldClientId:
				p.adcClientId = dcBase32Decode(val)
			case adcFieldSoftware:
				p.Client = val
			case adcFieldVersion:
				p.Version = val
			case adcFieldTlsFingerprint:
				p.adcFingerprint = val

			case adcFieldSupports:
				p.adcSupports = make(map[string]struct{})
				for _, feat := range strings.Split(val, ",") {
					p.adcSupports[feat] = struct{}{}
				}

			case adcFieldCategory:
				ct := atoui(val)
				p.IsBot = (ct & 1) != 0
				p.IsOperator = ((ct & 4) | (ct & 8) | (ct & 16)) != 0
			}
		}

		// a peer is active if it supports udp4, it exposes udp port and ip
		p.IsPassive = true
		if _, ok := p.adcSupports[adcSupportUdp4]; ok {
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
			h.handleHubInitialized()
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
		h.client.handleAdcSearchIncomingRequest(msg.SessionId, &msg.msgAdcKeySearchRequest)

	case *msgAdcFSearchRequest:
		if _, ok := msg.RequiredFeatures["TCP4"]; ok {
			if h.client.conf.IsPassive == true {
				dolog(LevelDebug, "[F warning] we are in passive and author requires active")
				return nil
			}
		}
		h.client.handleAdcSearchIncomingRequest(msg.SessionId, &msg.msgAdcKeySearchRequest)

	case *msgAdcDSearchResult:
		p := h.client.peerBySessionId(msg.AuthorId)
		if p == nil {
			return fmt.Errorf("search result with unknown author")
		}
		h.client.handleAdcSearchResult(false, p, &msg.msgAdcKeySearchResult)

	case *msgAdcDConnectToMe:
		p := h.client.peerBySessionId(msg.AuthorId)
		if p == nil {
			return fmt.Errorf("connecttome with unknown author")
		}
		if msg.Token == "" {
			return fmt.Errorf("connecttome with invalid token")
		}

		// invalid protocol
		if _, ok := map[string]struct{}{
			adcProtocolPlain:     {},
			adcProtocolEncrypted: {},
		}[msg.Protocol]; ok == false {
			h.conn.Write(&msgAdcDStatus{
				msgAdcTypeD{h.client.sessionId, msg.AuthorId},
				msgAdcKeyStatus{
					adcStatusWarning,
					adcCodeProtocolUnsupported,
					"Transfer protocol unsupported",
					map[string]string{
						adcFieldToken:    msg.Token,
						adcFieldProtocol: msg.Protocol,
					},
				},
			})
			return nil
		}

		// some clients send an ADCS request without checking whether we support it
		// or not. the same can happen for ADC. send back a status
		if (msg.Protocol == adcProtocolEncrypted &&
			h.client.conf.PeerEncryptionMode == DisableEncryption) ||
			(msg.Protocol == adcProtocolPlain &&
				h.client.conf.PeerEncryptionMode == ForceEncryption) {

			h.conn.Write(&msgAdcDStatus{
				msgAdcTypeD{h.client.sessionId, msg.AuthorId},
				msgAdcKeyStatus{adcStatusWarning, 41, "Transfer protocol unsupported",
					map[string]string{
						adcFieldToken:    msg.Token,
						adcFieldProtocol: msg.Protocol,
					},
				},
			})
			return nil
		}

		isEncrypted := (msg.Protocol == adcProtocolEncrypted)
		newConnPeer(h.client, isEncrypted, false, nil, p.Ip, msg.TcpPort, msg.Token)

	case *msgAdcDRevConnectToMe:
		p := h.client.peerBySessionId(msg.AuthorId)
		if p == nil {
			return fmt.Errorf("revconnecttome with unknown author")
		}
		if msg.Token == "" {
			return fmt.Errorf("revconnecttome with invalid token")
		}
		h.client.handlePeerRevConnectToMe(p, msg.Token)

	case *msgNmdcKeepAlive:

	case *msgNmdcZon:
		if h.client.conf.HubDisableCompression == true {
			return fmt.Errorf("zlib requested but zlib is disabled")
		}
		if err := h.conn.ReaderEnableZlib(); err != nil {
			return err
		}

	case *msgNmdcLock:
		if h.state != "connected" {
			return fmt.Errorf("[Lock] invalid state: %s", h.state)
		}
		h.state = "lock"

		// https://web.archive.org/web/20150323114734/http://wiki.gusari.org/index.php?title=$Supports
		// https://github.com/eiskaltdcpp/eiskaltdcpp/blob/master/dcpp/Nmdchub.cpp#L618
		features := map[string]struct{}{
			nmdcFeatureUserCommands: {},
			nmdcFeatureNoGetInfo:    {},
			nmdcFeatureNoHello:      {},
			nmdcFeatureUserIp:       {},
			nmdcFeatureTTHSearch:    {},
		}
		if h.client.conf.HubDisableCompression == false {
			features[nmdcFeatureZlibFull] = struct{}{}
		}
		// this must be provided, otherwise the final S is stripped from ConnectToMe
		if h.client.conf.PeerEncryptionMode != DisableEncryption {
			features[nmdcFeatureTls] = struct{}{}
		}

		h.conn.Write(&msgNmdcSupports{features})
		h.conn.Write(&msgNmdcKey{Key: nmdcComputeKey([]byte(msg.Lock))})
		h.conn.Write(&msgNmdcValidateNick{Nick: h.client.conf.Nick})

	case *msgNmdcValidateDenide:
		return fmt.Errorf("forbidden nickname")

	case *msgNmdcSupports:
		if h.state != "lock" {
			return fmt.Errorf("[Supports] invalid state: %s", h.state)
		}
		h.state = "preinitialized"

	// flexhub sends HubName just after lock
	// HubName can also be sent twice
	case *msgNmdcHubName:
		if h.state != "preinitialized" && h.state != "lock" {
			return fmt.Errorf("[HubName] invalid state: %s", h.state)
		}
		if h.client.OnHubInfo != nil {
			h.client.OnHubInfo(HubName, msg.Content)
		}
		dolog(LevelInfo, "[hub] [name] %s", msg.Content)

	case *msgNmdcHubTopic:
		if h.state != "preinitialized" && h.state != "initialized" {
			return fmt.Errorf("[HubTopic] invalid state: %s", h.state)
		}
		if h.client.OnHubInfo != nil {
			h.client.OnHubInfo(HubTopic, msg.Content)
		}
		dolog(LevelInfo, "[hub] [topic] %s", msg.Content)

	case *msgNmdcGetPass:
		if h.state != "preinitialized" {
			return fmt.Errorf("[GetPass] invalid state: %s", h.state)
		}
		h.passwordSent = true
		h.conn.Write(&msgNmdcMyPass{Pass: h.client.conf.Password})
		if _, ok := h.uniqueCmds["GetPass"]; ok {
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
		if _, ok := h.uniqueCmds["LoggedIn"]; ok {
			return fmt.Errorf("LoggedIn sent twice")
		}
		h.uniqueCmds["LoggedIn"] = struct{}{}

	case *msgNmdcHello:
		if h.state != "preinitialized" {
			return fmt.Errorf("[Hello] invalid state: %s", h.state)
		}
		if _, ok := h.uniqueCmds["Hello"]; ok {
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
			p = &Peer{Nick: msg.Nick}
		}

		p.Description = msg.Description
		p.Email = msg.Email
		p.ShareSize = msg.ShareSize
		p.nmdcConnection = msg.Connection
		p.nmdcStatusByte = msg.StatusByte

		// client, version, mode are in the tag part of MyInfo (i.e. <>)
		// that can be deliberately hidden by hub
		if msg.Client != "" {
			p.Client = msg.Client
		}
		if msg.Version != "" {
			p.Version = msg.Version
		}
		if msg.Mode != "" {
			p.IsPassive = (msg.Mode == "P")
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
		for peer, ip := range msg.Ips {
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

		for _, p := range h.client.peers {
			_, isOp := msg.Ops[p.Nick]
			if isOp != p.IsOperator {
				p.IsOperator = isOp
				h.client.handlePeerUpdated(p)
			}
		}

		// switch to initialized
		if h.state != "initialized" {
			h.state = "initialized"
			h.handleHubInitialized()
		}

	case *msgNmdcBotList:
		if h.state != "initialized" {
			return fmt.Errorf("[BotList] invalid state: %s", h.state)
		}

		for _, p := range h.client.peers {
			_, isBot := msg.Bots[p.Nick]
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
		return fmt.Errorf("received force move (%+v)", msg)

	case *msgNmdcSearchRequest:
		// searches can be received even before initialization; ignore them
		if h.state == "initialized" {
			h.client.handleNmdcSearchIncomingRequest(msg)
		}

	case *msgNmdcSearchResult:
		if h.state != "initialized" {
			return fmt.Errorf("[SearchResult] invalid state: %s", h.state)
		}
		p := h.client.peerByNick(msg.Nick)
		if p != nil {
			h.client.handleNmdcSearchResult(false, p, msg)
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
			h.client.handlePeerRevConnectToMe(p, "")
		}

	case *msgNmdcPublicChat:
		p := h.client.peerByNick(msg.Author)
		if p == nil { // create a dummy peer if not found
			p = &Peer{Nick: msg.Author}
		}
		h.client.handlePublicMessage(p, msg.Content)

	case *msgNmdcPrivateChat:
		p := h.client.peerByNick(msg.Author)
		if p == nil { // create a dummy peer if not found
			p = &Peer{Nick: msg.Author}
		}
		h.client.handlePrivateMessage(p, msg.Content)

	default:
		return fmt.Errorf("unhandled: %T %+v", msgi, msgi)
	}
	return nil
}

func (h *connHub) handleHubInitialized() {
	dolog(LevelInfo, "[hub] initialized, %d peers", len(h.client.peers))
	if h.client.OnHubConnected != nil {
		h.client.OnHubConnected()
	}
}
