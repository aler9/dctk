package dctk

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"
	godctiger "github.com/aler9/go-dc/tiger"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/proto"
	"github.com/aler9/dctk/pkg/tiger"
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

type hubConn struct {
	client             *Client
	name               string
	terminateRequested bool
	terminate          chan struct{}
	state              hubConnState
	conn               proto.Conn
	passwordSent       bool
	uniqueCmds         map[string]struct{}
}

func newHubConn(client *Client) error {
	client.hubConn = &hubConn{
		client:     client,
		terminate:  make(chan struct{}),
		state:      hubDisconnected,
		uniqueCmds: make(map[string]struct{}),
	}
	return nil
}

// HubConnect starts the connection to the hub. It must be called only when
// HubManualConnect is true.
func (c *Client) HubConnect() {
	if c.hubConn.state != hubDisconnected {
		return
	}
	c.hubConn.state = hubConnecting
	c.wg.Add(1)
	go c.hubConn.do()
}

func (h *hubConn) close() {
	if h.terminateRequested {
		return
	}
	h.terminateRequested = true
	close(h.terminate)
}

func (h *hubConn) do() {
	defer h.client.wg.Done()

	err := func() error {
		// resolve hub ip
		ips, err := net.LookupIP(h.client.hubHostname)
		if err != nil {
			return err
		}
		h.client.hubSolvedIP = ips[0].String()

		// connect to hub
		ce := newConnEstablisher(
			fmt.Sprintf("%s:%d", h.client.hubSolvedIP, h.client.hubPort),
			10*time.Second, h.client.conf.HubConnTries)

		select {
		case <-h.terminate:
			return proto.ErrorTerminated
		case <-ce.Wait:
		}

		if ce.Error != nil {
			return ce.Error
		}

		// hub connected
		rawconn := ce.Conn
		if h.client.hubIsEncrypted {
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
				log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] negotiated %q", st.NegotiatedProtocol)
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
		protoName := ""
		if h.client.protoIsAdc() {
			protoName = "adc"
			h.conn = proto.NewAdcConn(h.client.conf.LogLevel, "h", rawconn, false, true)
		} else {
			protoName = "nmdc"
			h.conn = proto.NewNmdcConn(h.client.conf.LogLevel, "h", rawconn, false, true)
		}
		if h.client.OnHubProto != nil {
			h.client.OnHubProto(protoName)
		}

		if !h.client.conf.HubDisableKeepAlive {
			keepaliver := newHubKeepAliver(h)
			defer keepaliver.Close()
		}

		log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] connected (%s)", rawconn.RemoteAddr())

		if h.client.protoIsAdc() {
			features := adc.ModFeatures{
				adc.FeaBAS0: true,
				adc.FeaBASE: true,
				adc.FeaTIGR: true,
				adc.FeaUCM0: true,
			}
			if !h.client.conf.HubDisableCompression {
				features[adc.FeaZLIF] = true
			}
			h.conn.Write(&proto.AdcHSupports{ //nolint:govet
				&adc.HubPacket{},
				&adc.Supported{features}, //nolint:govet
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
			return proto.ErrorTerminated

		case err := <-readDone:
			h.conn.Close()
			return err
		}
	}()

	h.client.Safe(func() {
		if !h.terminateRequested {
			log.Log(h.client.conf.LogLevel, log.LevelInfo, "ERR: %s", err)

			if h.client.OnHubError != nil {
				h.client.OnHubError(err)
			}
		}

		log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] disconnected")

		// close client too
		h.client.Close()
	})
}

func (h *hubConn) handleMessage(msgi proto.MsgDecodable) error {
	switch msg := msgi.(type) {
	case *proto.AdcKeepAlive:

	case *proto.AdcIZon:
		if h.client.conf.HubDisableCompression {
			return fmt.Errorf("zlib requested but zlib is disabled")
		}
		if err := h.conn.ReaderEnableZlib(); err != nil {
			return err
		}

	case *proto.AdcIStatus:
		switch msg.Msg.Sev {
		case adc.Success:

		case adc.Recoverable:
			log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] [WARN] %s (%d)", msg.Msg.Msg, msg.Msg.Code)

		case adc.Fatal:
			return fmt.Errorf("fatal: %s (%d)", msg.Msg.Msg, msg.Msg.Code)
		}

	case *proto.AdcISupports:
		if h.state != hubConnected {
			return fmt.Errorf("[Supports] invalid state: %s", h.state)
		}
		h.state = hubSupports

	case *proto.AdcISessionID:
		if h.state != hubSupports {
			return fmt.Errorf("[SessionId] invalid state: %s", h.state)
		}
		h.state = hubSessionID
		h.client.adcSessionID = msg.Msg.SID
		h.client.sendInfos(true)

	case *proto.AdcIInfos:
		onHubInfo := func(k HubField, v string) {
			if h.client.OnHubInfo != nil {
				h.client.OnHubInfo(k, v)
			}
			log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] [%s] %s", k, v)
		}

		if msg.Msg.Name != "" {
			h.name = msg.Msg.Name
			onHubInfo(HubName, msg.Msg.Name)
		}
		if msg.Msg.Application != "" {
			onHubInfo(HubSoftware, msg.Msg.Application)
		}
		if msg.Msg.Version != "" {
			onHubInfo(HubVersion, msg.Msg.Version)
		}
		if msg.Msg.Desc != "" {
			onHubInfo(HubDescription, msg.Msg.Desc)
		}

	case *proto.AdcIMsg:
		h.client.handlePublicMessage(&Peer{Nick: h.name}, msg.Msg.Text)
		log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] %s", msg.Msg.Text)

	case *proto.AdcIGetPass:
		if h.state != hubSessionID {
			return fmt.Errorf("[Sup] invalid state: %s", h.state)
		}
		h.state = hubGetPass

		hasher := tiger.NewHash()
		hasher.Write([]byte(h.client.conf.Password))
		hasher.Write(msg.Msg.Salt)
		var data godctiger.Hash
		hasher.Sum(data[:0])

		h.passwordSent = true
		h.conn.Write(&proto.AdcHPass{ //nolint:govet
			&adc.HubPacket{},
			&adc.Password{Hash: data},
		})

	case *proto.AdcBInfos:
		exists := true
		p := h.client.peerBySessionID(msg.Pkt.ID)
		if p == nil {
			exists = false

			// adcFieldName is mandatory for peer creation
			if msg.Msg.Name == "" {
				return fmt.Errorf("peer name not provided")
			}
			if h.client.peerByNick(msg.Msg.Name) != nil {
				return fmt.Errorf("a peer with this name already exists")
			}

			p = &Peer{
				Nick:         msg.Msg.Name,
				adcSessionID: msg.Pkt.ID,
			}
		}

		// every field is optional
		if msg.Msg.Desc != "" {
			p.Description = msg.Msg.Desc
		}
		if msg.Msg.Email != "" {
			p.Email = msg.Msg.Email
		}
		if msg.Msg.ShareSize != 0 {
			p.ShareSize = uint64(msg.Msg.ShareSize)
		}
		if msg.Msg.Ip4 != "" {
			p.IP = msg.Msg.Ip4
		}
		if msg.Msg.Udp4 != 0 {
			p.adcUDPPort = uint(msg.Msg.Udp4)
		}
		var zeroCID adc.CID
		if msg.Msg.Id != zeroCID {
			p.adcClientID = msg.Msg.Id
		}
		if msg.Msg.Application != "" {
			p.Client = msg.Msg.Application
		}
		if msg.Msg.Version != "" {
			p.Version = msg.Msg.Version
		}
		if msg.Msg.KP != "" {
			p.adcFingerprint = msg.Msg.KP
		}
		if len(msg.Msg.Features) > 0 {
			p.adcFeatures = msg.Msg.Features
		}
		if adc.UserTypeBot != 0 {
			p.IsBot = (msg.Msg.Type & adc.UserTypeBot) != 0
			p.IsOperator = (msg.Msg.Type & adc.UserTypeOperator) != 0
		}

		// a peer is active if it supports udp4, exposes udp port and ip
		p.IsPassive = true
		if h.client.peerSupportsAdc(p, adc.FeaUDP4) && p.IP != "" && p.adcUDPPort != 0 {
			p.IsPassive = false
		}

		if !exists {
			h.client.handlePeerConnected(p)
		} else {
			h.client.handlePeerUpdated(p)
		}

	case *proto.AdcIQuit:
		// self quit, used instead of ForceMove
		if msg.Msg.ID == h.client.adcSessionID {
			return fmt.Errorf("received Quit message: %s", msg.Msg.Message)
		}
		// peer quit
		p := h.client.peerBySessionID(msg.Msg.ID)
		if p != nil {
			h.client.handlePeerDisconnected(p)
		}

	case *proto.AdcICommand:
		// switch to initialized
		if h.state != hubInitialized {
			h.state = hubInitialized
			h.handleHubInitialized()
		}

	case *proto.AdcBMessage:
		p := h.client.peerBySessionID(msg.Pkt.ID)
		if p == nil {
			return fmt.Errorf("public message with unknown author")
		}
		h.client.handlePublicMessage(p, msg.Msg.Text)

	case *proto.AdcDMessage:
		p := h.client.peerBySessionID(msg.Pkt.ID)
		if p == nil {
			return fmt.Errorf("private message with unknown author")
		}
		h.client.handlePrivateMessage(p, msg.Msg.Text)

	case *proto.AdcBSearchRequest:
		h.client.handleAdcSearchIncomingRequest(msg.Pkt.ID, msg.Msg)

	case *proto.AdcFSearchRequest:
		hasFeature := func(f adc.Feature) bool {
			for _, s := range msg.Pkt.Sel {
				if s.Fea == f {
					return true
				}
			}
			return false
		}

		if h.client.conf.IsPassive && hasFeature(adc.FeaTCP4) {
			log.Log(h.client.conf.LogLevel, log.LevelDebug, "we are in passive and author requires active")
			return nil
		}

		h.client.handleAdcSearchIncomingRequest(msg.Pkt.ID, msg.Msg)

	case *proto.AdcDSearchResult:
		p := h.client.peerBySessionID(msg.Pkt.ID)
		if p == nil {
			return fmt.Errorf("search result with unknown author")
		}
		h.client.handleAdcSearchResult(false, p, msg.Msg)

	case *proto.AdcDConnectToMe:
		p := h.client.peerBySessionID(msg.Pkt.ID)
		if p == nil {
			return fmt.Errorf("connecttome with unknown author")
		}
		if msg.Msg.Token == "" {
			return fmt.Errorf("connecttome with invalid token")
		}

		// invalid protocol
		if _, ok := map[string]struct{}{
			adc.ProtoADC:  {},
			adc.ProtoADCS: {},
		}[msg.Msg.Proto]; !ok {
			h.conn.Write(&proto.AdcDStatus{ //nolint:govet
				&adc.DirectPacket{ID: h.client.adcSessionID, To: msg.Pkt.ID},
				&adc.Status{
					Sev:  adc.Recoverable,
					Code: proto.AdcCodeProtocolUnsupported,
					Msg:  "Transfer protocol unsupported",
					// TODO: add additional fields
					/*map[string]string{
						adcFieldToken:    msg.Msg.Token,
						adcFieldProtocol: msg.Msg.Protocol,
					},*/
				},
			})
			return nil
		}

		// some clients send an ADCS request without checking whether we support it
		// or not. the same can happen for ADC. send back a status
		if (msg.Msg.Proto == adc.ProtoADCS &&
			h.client.conf.PeerEncryptionMode == DisableEncryption) ||
			(msg.Msg.Proto == adc.ProtoADC &&
				h.client.conf.PeerEncryptionMode == ForceEncryption) {

			h.conn.Write(&proto.AdcDStatus{ //nolint:govet
				&adc.DirectPacket{ID: h.client.adcSessionID, To: msg.Pkt.ID},
				&adc.Status{
					Sev:  adc.Recoverable,
					Code: proto.AdcCodeProtocolUnsupported,
					Msg:  "Transfer protocol unsupported",
					// TODO: add additional fields
					/*map[string]string{
						adcFieldToken:    msg.Msg.Token,
						adcFieldProtocol: msg.Msg.Protocol,
					},*/
				},
			})
			return nil
		}

		newPeerConn(h.client, (msg.Msg.Proto == adc.ProtoADCS), false, nil, p.IP, uint(msg.Msg.Port), msg.Msg.Token)

	case *proto.AdcDRevConnectToMe:
		p := h.client.peerBySessionID(msg.Pkt.ID)
		if p == nil {
			return fmt.Errorf("revconnecttome with unknown author")
		}
		if msg.Msg.Token == "" {
			return fmt.Errorf("revconnecttome with invalid token")
		}
		h.client.handlePeerRevConnectToMe(p, msg.Msg.Token)

	case *proto.NmdcKeepAlive:

	case *nmdc.ZOn:
		if h.client.conf.HubDisableCompression {
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
			nmdc.ExtUserCommand,
			nmdc.ExtNoGetINFO,
			nmdc.ExtNoHello,
			nmdc.ExtUserIP2,
			nmdc.ExtTTHSearch,
		}
		if !h.client.conf.HubDisableCompression {
			features = append(features, nmdc.ExtZPipe0)
		}
		// this must be provided, otherwise the final S is stripped from ConnectToMe
		if h.client.conf.PeerEncryptionMode != DisableEncryption {
			features = append(features, nmdc.ExtTLS)
		}

		h.conn.Write(&nmdc.Supports{features}) //nolint:govet
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
		log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] [name] %s", string(msg.String))

	case *nmdc.HubTopic:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[HubTopic] invalid state: %s", h.state)
		}
		if h.client.OnHubInfo != nil {
			h.client.OnHubInfo(HubTopic, msg.Text)
		}
		log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] [topic] %s", msg.Text)

	case *nmdc.GetPass:
		if h.state != hubPreInitialized {
			return fmt.Errorf("[GetPass] invalid state: %s", h.state)
		}
		h.passwordSent = true
		h.conn.Write(&nmdc.MyPass{nmdc.String(h.client.conf.Password)}) //nolint:govet
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

		if !exists {
			h.client.handlePeerConnected(p)
		} else {
			h.client.handlePeerUpdated(p)
		}

	case *nmdc.UserIP:
		if h.state != hubPreInitialized && h.state != hubInitialized {
			return fmt.Errorf("[UserIP] invalid state: %s", h.state)
		}

		// we do not use UserIP to get our own ip, but only to get other
		// ips of other peers
		for _, entry := range msg.List {
			// update peer
			if p := h.client.peerByNick(entry.Name); p != nil {
				p.IP = entry.IP
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
		matches := proto.ReNmdcAddress.FindStringSubmatch(msg.Address)
		if matches == nil {
			return fmt.Errorf("invalid address")
		}
		ip, port := matches[1], atoui(matches[2])

		if h.state != hubInitialized && h.state != hubPreInitialized {
			return fmt.Errorf("[ConnectToMe] invalid state: %s", h.state)
		}
		if msg.Secure && h.client.conf.PeerEncryptionMode == DisableEncryption {
			log.Log(h.client.conf.LogLevel, log.LevelInfo, "received encrypted connect to me request but encryption is disabled, skipping")
		} else if !msg.Secure && h.client.conf.PeerEncryptionMode == ForceEncryption {
			log.Log(h.client.conf.LogLevel, log.LevelInfo, "received plain connect to me request but encryption is forced, skipping")
		} else {
			newPeerConn(h.client, msg.Secure, false, nil, ip, port, "")
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

func (h *hubConn) handleHubInitialized() {
	log.Log(h.client.conf.LogLevel, log.LevelInfo, "[hub] initialized, %d peers", len(h.client.peers))
	if h.client.OnHubConnected != nil {
		h.client.OnHubConnected()
	}
}
