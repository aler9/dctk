package dctoolkit

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gswly/go-dc/nmdc"
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

type hubConnState int

const (
	hubDisconnected = hubConnState(iota)
	hubConnecting
	hubConnected
	hubSupports
	hubSessionID
	hubGetPass
	hubInitialized
	hubPreInitialized
	hubLock
)

func (s hubConnState) String() string {
	switch s {
	case hubDisconnected:
		return "disconnected"
	case hubConnecting:
		return "connecting"
	case hubConnected:
		return "connected"
	case hubSupports:
		return "supports"
	case hubSessionID:
		return "sessionid"
	case hubGetPass:
		return "getpass"
	case hubInitialized:
		return "initialized"
	case hubPreInitialized:
		return "preinitialized"
	case hubLock:
		return "lock"
	default:
		return "state(" + strconv.Itoa(int(s)) + ")"
	}
}

type connHub struct {
	client             *Client
	name               string
	terminateRequested bool
	terminate          chan struct{}
	state              hubConnState
	conn               protocol
	passwordSent       bool
	uniqueCmds         map[string]struct{}
}

func newConnHub(client *Client) error {
	client.connHub = &connHub{
		client:     client,
		terminate:  make(chan struct{}, 1),
		state:      hubDisconnected,
		uniqueCmds: make(map[string]struct{}),
	}
	return nil
}

// HubConnect starts the connection to the hub. It must be called only when
// HubManualConnect is true.
func (c *Client) HubConnect() {
	if c.connHub.state != hubDisconnected {
		return
	}
	c.connHub.state = hubConnecting
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
			tlsconn := tls.Client(rawconn, &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{"adc", "nmdc"},
			})
			rawconn = tlsconn
			err = tlsconn.Handshake()
			if err != nil {
				tlsconn.Close()
				return err
			}
			st := tlsconn.ConnectionState()
			if h.client.OnHubTLS != nil {
				h.client.OnHubTLS(st)
			}
			if st.NegotiatedProtocol != "" {
				dolog(LevelInfo, "[hub] negotiated %q", st.NegotiatedProtocol)
				// ALPN negotiation
				switch st.NegotiatedProtocol {
				case "adc":
					h.client.setProto(protocolADC)
				case "nmdc":
					h.client.setProto(protocolNMDC)
				}
			}
		}

		// do not use read timeout since hub does not send data continuously
		proto := ""
		if h.client.protoIsAdc() {
			proto = "adc"
			h.conn = newProtocolAdc("h", rawconn, false, true)
		} else {
			proto = "nmdc"
			h.conn = newProtocolNmdc("h", rawconn, false, true)
		}
		if h.client.OnHubProto != nil {
			h.client.OnHubProto(proto)
		}

		if h.client.conf.HubDisableKeepAlive == false {
			keepaliver := newHubKeepAliver(h)
			defer keepaliver.Close()
		}

		dolog(LevelInfo, "[hub] connected (%s)", rawconn.RemoteAddr())

		if h.client.protoIsAdc() {
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
			h.state = hubConnected
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
		if h.state != hubConnected {
			return fmt.Errorf("[Supports] invalid state: %s", h.state)
		}
		h.state = hubSupports

	case *msgAdcISessionId:
		if h.state != hubSupports {
			return fmt.Errorf("[SessionId] invalid state: %s", h.state)
		}
		h.state = hubSessionID
		h.client.sessionId = msg.Sid
		h.client.sendInfos(true)

	case *msgAdcIInfos:
		for key, val := range msg.Fields {
			var klabel HubField
			switch key {
			case adcFieldName:
				klabel = HubName
				h.name = val
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
		h.client.handlePublicMessage(&Peer{Nick: h.name}, msg.Content)
		dolog(LevelInfo, "[hub] %s", msg.Content)

	case *msgAdcIGetPass:
		if h.state != hubSessionID {
			return fmt.Errorf("[Sup] invalid state: %s", h.state)
		}
		h.state = hubGetPass

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
		if h.state != hubInitialized {
			h.state = hubInitialized
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

	case *nmdcKeepAlive:

	case *nmdc.ZOn:
		if h.client.conf.HubDisableCompression == true {
			return fmt.Errorf("zlib requested but zlib is disabled")
		}
		if err := h.conn.ReaderEnableZlib(); err != nil {
			return err
		}

	case *nmdc.Lock:
		if h.state != hubConnected {
			return fmt.Errorf("[Lock] invalid state: %s", h.state)
		}
		h.state = hubLock

		// https://web.archive.org/web/20150323114734/http://wiki.gusari.org/index.php?title=$Supports
		// https://github.com/eiskaltdcpp/eiskaltdcpp/blob/master/dcpp/Nmdchub.cpp#L618
		features := []string{
			nmdcFeatureUserCommands,
			nmdcFeatureNoGetInfo,
			nmdcFeatureNoHello,
			nmdcFeatureUserIp,
			nmdcFeatureTTHSearch,
		}
		if h.client.conf.HubDisableCompression == false {
			features = append(features, nmdcFeatureZlibFull)
		}
		// this must be provided, otherwise the final S is stripped from ConnectToMe
		if h.client.conf.PeerEncryptionMode != DisableEncryption {
			features = append(features, nmdcFeatureTls)
		}

		h.conn.Write(&nmdc.Supports{features})
		h.conn.Write(msg.Key())
		h.conn.Write(&nmdc.ValidateNick{Name: nmdc.Name(h.client.conf.Nick)})

	case *nmdc.ValidateDenide:
		return fmt.Errorf("forbidden nickname")

	case *nmdc.Supports:
		if h.state != hubLock {
			return fmt.Errorf("[Supports] invalid state: %s", h.state)
		}
		h.state = hubPreInitialized

	// flexhub sends HubName just after lock
	// HubName can also be sent twice
	case *nmdc.HubName:
		if h.state != hubPreInitialized && h.state != hubLock {
			return fmt.Errorf("[HubName] invalid state: %s", h.state)
		}
		if h.client.OnHubInfo != nil {
			h.client.OnHubInfo(HubName, string(msg.String))
		}
		dolog(LevelInfo, "[hub] [name] %s", string(msg.String))

	case *nmdc.HubTopic:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[HubTopic] invalid state: %s", h.state)
		}
		if h.client.OnHubInfo != nil {
			h.client.OnHubInfo(HubTopic, msg.Text)
		}
		dolog(LevelInfo, "[hub] [topic] %s", msg.Text)

	case *nmdc.GetPass:
		if h.state != hubPreInitialized {
			return fmt.Errorf("[GetPass] invalid state: %s", h.state)
		}
		h.passwordSent = true
		h.conn.Write(&nmdc.MyPass{nmdc.String(h.client.conf.Password)})
		if _, ok := h.uniqueCmds["GetPass"]; ok {
			return fmt.Errorf("GetPass sent twice")
		}
		h.uniqueCmds["GetPass"] = struct{}{}

	case *nmdc.BadPass:
		return fmt.Errorf("wrong password")

	case *nmdc.HubIsFull:
		return fmt.Errorf("hub is full")

	case *nmdc.LogedIn:
		if h.state != hubPreInitialized {
			return fmt.Errorf("[LoggedIn] invalid state: %s", h.state)
		}
		if _, ok := h.uniqueCmds["LoggedIn"]; ok {
			return fmt.Errorf("LoggedIn sent twice")
		}
		h.uniqueCmds["LoggedIn"] = struct{}{}

	case *nmdc.Hello:
		if h.state != hubPreInitialized {
			return fmt.Errorf("[Hello] invalid state: %s", h.state)
		}
		if _, ok := h.uniqueCmds["Hello"]; ok {
			return fmt.Errorf("Hello sent twice")
		}
		h.uniqueCmds["Hello"] = struct{}{}

		// The last version of the Neo-Modus client was 1,0091 and is what is commonly used by current clients
		// https://github.com/eiskaltdcpp/eiskaltdcpp/blob/1e72256ac5e8fe6735f81bfbc3f9d90514ada578/dcpp/NmdcHub.h#L119
		h.conn.Write(&nmdc.Version{Vers: "1,0091"})
		h.client.sendInfos(true)
		h.conn.Write(&nmdc.GetNickList{})

	case *nmdc.MyINFO:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[MyInfo] invalid state: %s", h.state)
		}
		exists := true
		p := h.client.peerByNick(msg.Name)
		if p == nil {
			exists = false
			p = &Peer{Nick: msg.Name}
		}

		p.Description = msg.Desc
		p.Email = msg.Email
		p.ShareSize = msg.ShareSize
		p.nmdcConnection = msg.Conn
		p.nmdcFlag = msg.Flag

		// client, version, mode are in the tag part of MyInfo (i.e. <>)
		// that can be deliberately hidden by hub
		p.Client = msg.Client.Name
		p.Version = msg.Client.Version
		p.IsPassive = (msg.Mode == nmdc.UserModePassive)

		if exists == false {
			h.client.handlePeerConnected(p)
		} else {
			h.client.handlePeerUpdated(p)
		}

	case *nmdc.UserIP:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[UserIp] invalid state: %s", h.state)
		}

		// we do not use UserIp to get our own ip, but only to get other
		// ips of other peers
		for _, entry := range msg.List {
			// update peer
			if p := h.client.peerByNick(entry.Name); p != nil {
				p.Ip = entry.IP
				h.client.handlePeerUpdated(p)
			}
		}

	case *nmdc.OpList:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[OpList] invalid state: %s", h.state)
		}

		updatedPeers := make(map[string]struct{})
		for _, p := range h.client.peers {
			if p.IsOperator {
				updatedPeers[p.Nick] = struct{}{}
				p.IsOperator = false
			}
		}

		for _, name := range msg.Names {
			h.client.peers[name].IsOperator = true
			if _, ok := updatedPeers[name]; ok {
				delete(updatedPeers, name)
			} else {
				updatedPeers[name] = struct{}{}
			}
		}

		for name := range updatedPeers {
			h.client.handlePeerUpdated(h.client.peers[name])
		}

		// switch to initialized
		if h.state != hubInitialized {
			h.state = hubInitialized
			h.handleHubInitialized()
		}

	case *nmdc.BotList:
		if h.state != hubInitialized {
			return fmt.Errorf("[BotList] invalid state: %s", h.state)
		}

		updatedPeers := make(map[string]struct{})
		for _, p := range h.client.peers {
			if p.IsBot {
				updatedPeers[p.Nick] = struct{}{}
				p.IsBot = false
			}
		}

		for _, name := range msg.Names {
			h.client.peers[name].IsBot = true
			if _, ok := updatedPeers[name]; ok {
				delete(updatedPeers, name)
			} else {
				updatedPeers[name] = struct{}{}
			}
		}

		for name := range updatedPeers {
			h.client.handlePeerUpdated(h.client.peers[name])
		}

	case *nmdc.UserCommand:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[UserCommand] invalid state: %s", h.state)
		}

	case *nmdc.Quit:
		if h.state != hubInitialized {
			return fmt.Errorf("[Quit] invalid state: %s", h.state)
		}
		p := h.client.peerByNick(string(msg.Name))
		if p != nil {
			h.client.handlePeerDisconnected(p)
		}

	case *nmdc.ForceMove:
		// means disconnect and reconnect to provided address
		// we just disconnect
		return fmt.Errorf("received force move (%+v)", msg)

	case *nmdc.Search:
		// searches can be received even before initialization; ignore them
		if h.state == hubInitialized {
			h.client.handleNmdcSearchIncomingRequest(msg)
		}

	case *nmdc.SR:
		if h.state != hubInitialized {
			return fmt.Errorf("[SearchResult] invalid state: %s", h.state)
		}
		h.client.handleNmdcSearchResult(false, msg)

	case *nmdc.ConnectToMe:
		matches := reNmdcAddress.FindStringSubmatch(msg.Address)
		if matches == nil {
			return fmt.Errorf("invalid address")
		}
		ip, port := matches[1], atoui(matches[2])

		if h.state != hubInitialized && h.state != hubPreInitialized {
			return fmt.Errorf("[ConnectToMe] invalid state: %s", h.state)
		}
		if msg.Secure && h.client.conf.PeerEncryptionMode == DisableEncryption {
			dolog(LevelInfo, "received encrypted connect to me request but encryption is disabled, skipping")
		} else if !msg.Secure && h.client.conf.PeerEncryptionMode == ForceEncryption {
			dolog(LevelInfo, "received plain connect to me request but encryption is forced, skipping")
		} else {
			newConnPeer(h.client, msg.Secure, false, nil, ip, port, "")
		}

	case *nmdc.RevConnectToMe:
		if h.state != hubInitialized && h.state != hubPreInitialized {
			return fmt.Errorf("[RevConnectToMe] invalid state: %s", h.state)
		}
		p := h.client.peerByNick(msg.From)
		if p != nil {
			h.client.handlePeerRevConnectToMe(p, "")
		}

	case *nmdc.ChatMessage:
		p := h.client.peerByNick(msg.Name)
		if p == nil { // create a dummy peer if not found
			p = &Peer{Nick: msg.Name}
		}
		h.client.handlePublicMessage(p, msg.Text)

	case *nmdc.PrivateMessage:
		p := h.client.peerByNick(msg.From)
		if p == nil { // create a dummy peer if not found
			p = &Peer{Nick: msg.From}
		}
		h.client.handlePrivateMessage(p, msg.Text)

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
