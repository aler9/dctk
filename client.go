/*
Package dctk implements the client part of the Direct Connect
peer-to-peer system (ADC and NMDC protocols) in the Go programming language.
It allows the creation of clients that can interact with hubs and other
clients, and can be used as backend to user interfaces or automatic bots.

Basic example (more are available at https://github.com/aler9/dctk/tree/master/examples)

  package main

  import (
  	"fmt"
  	"github.com/aler9/dctk"
  )

  func main() {
  	client, err := dctk.NewClient(dctk.ClientConf{
  		HubUrl:     "nmdc://hubip:411",
  		Nick:       "mynick",
  		TcpPort:    3009,
  		UdpPort:    3009,
  		TcpTlsPort: 3010,
  	})
  	if err != nil {
  		panic(err)
  	}

  	client.OnHubConnected = func() {
  		fmt.Println("connected to hub")
  	}

  	client.Run()
  }
*/
package dctk

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/go-dc/adc"
	atypes "github.com/aler9/go-dc/adc/types"
	"github.com/aler9/go-dc/nmdc"
	"github.com/aler9/go-dc/types"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/proto"
	"github.com/aler9/dctk/pkg/tiger"
)

const (
	_PUBLIC_IP_PROVIDER = "http://checkip.dyndns.org/"
)

var rePublicIp = regexp.MustCompile("(" + proto.ReStrIp + ")")

// EncryptionMode contains the options regarding encryption.
type EncryptionMode int

const (
	// PreferEncryption uses encryption when the two peers both support it
	PreferEncryption EncryptionMode = iota
	// DisableEncryption disables competely encryption
	DisableEncryption
	// ForceEncryption forces encryption and block interaction with peers that
	// do not support encrypton
	ForceEncryption
)

type transfer interface {
	isTransfer()
	Close()
	handleExit(error)
}

// ClientConf allows to configure a client.
type ClientConf struct {
	// verbosity of the library
	LogLevel log.Level

	// turns on passive mode: it is not necessary anymore to open TcpPort, UdpPort
	// and TcpTlsPort but functionalities are limited
	IsPassive bool
	// (optional) an explicit ip, instead of the one obtained automatically
	Ip string
	// these are the 3 ports needed for active mode. They must be accessible from the
	// internet, so any router/firewall in between must be configured
	TcpPort    uint
	UdpPort    uint
	TcpTlsPort uint

	// the maximum number of file to download in parallel. When this number is
	// exceeded, the other downloads are queued and started when a slot becomes available
	DownloadMaxParallel uint
	// the maximum number of file to upload in parallel
	UploadMaxParallel uint

	// set the policy regarding encryption with other peers. See EncryptionMode for options
	PeerEncryptionMode EncryptionMode

	// The hub url in the format protocol://address:port
	// supported protocols are adc, adcs, nmdc and nmdcs
	HubUrl string
	// how many times attempting a connection with hub before giving up
	HubConnTries uint
	// if turned on, connection to hub is not automatic and HubConnect() must be
	// called manually
	HubManualConnect bool

	// the nickname to use in the hub and with other peers
	Nick string
	// the password associated with the nick, if requested by the hub
	Password string
	// the private ID of the user (ADC only)
	PID atypes.PID
	// an email, optional
	Email string
	// a description, optional
	Description string
	// the maximum upload speed in bytes/sec. It is not really applied, but is sent to the hub
	UploadMaxSpeed uint
	// these are used to identify the software. By default they mimic DC++
	ClientString  string
	ClientVersion string
	PkValue       string
	ListGenerator string

	// options useful only for debugging purposes
	HubDisableCompression  bool
	PeerDisableCompression bool
	HubDisableKeepAlive    bool
}

type protocolName uint32

const (
	protocolNMDC = protocolName(iota)
	protocolADC
)

// Client represents a local client.
type Client struct {
	conf               ClientConf
	mutex              sync.Mutex
	wg                 sync.WaitGroup
	proto              protocolName // atomic
	terminateRequested bool
	terminate          chan struct{}
	hubIsEncrypted     bool
	hubHostname        string
	hubPort            uint
	hubSolvedIp        string
	ip                 string
	shareIndexer       *shareIndexer
	shareRoots         map[string]string
	shareTree          map[string]*shareDirectory
	shareCount         uint
	shareSize          uint64
	fileList           []byte
	listenerTcp        *listenerTcp
	tcpTlsListener     *listenerTcp
	listenerUdp        *listenerUdp
	hubConn            *hubConn
	// we follow the ADC way to handle IDs, even when using NMDC
	privateId             atypes.PID
	clientId              atypes.CID
	adcSessionId          atypes.SID
	adcFingerprint        string
	peers                 map[string]*Peer
	downloadSlotAvail     uint
	uploadSlotAvail       uint
	peerConns             map[*peerConn]struct{}
	peerConnsByKey        map[nickDirectionPair]*peerConn
	transfers             map[transfer]struct{}
	activeDownloadsByPeer map[string]*Download

	// OnInitialized is called just after client initialization, before connecting to the hub
	OnInitialized func()
	// OnShareIndexed is called every time the share indexer has finished indexing the client share
	OnShareIndexed func()
	// OnHubConnected is called when the connection between client and hub has been established
	OnHubConnected func()
	// OnHubError is called when a critical error happens
	OnHubError func(err error)
	// OnHubInfo is called when an information about the hub is received
	OnHubInfo func(field HubField, value string)
	// OnHubTLS is called when a TLS connection with a hub is established
	OnHubTLS func(st tls.ConnectionState)
	// OnHubProto is called when a protocol for the hub is selected
	OnHubProto func(proto string)
	// OnPeerConnected is called when a peer connects to the hub
	OnPeerConnected func(p *Peer)
	// OnPeerUpdated is called when a peer has just updated its informations
	OnPeerUpdated func(p *Peer)
	// OnPeerDisconnected is called when a peer disconnects from the hub
	OnPeerDisconnected func(p *Peer)
	// OnMessagePublic is called when someone writes in the hub public chat.
	// When using ADC, it is also called when the hub sends a message.
	OnMessagePublic func(p *Peer, content string)
	// OnMessagePrivate is called when a private message has been received
	OnMessagePrivate func(p *Peer, content string)
	// OnSearchResult is called when a search result has been received
	OnSearchResult func(r *SearchResult)
	// OnDownloadSuccessful is called when a given download has finished
	OnDownloadSuccessful func(d *Download)
	// OnDownloadError is called when a given download has failed
	OnDownloadError func(d *Download)
}

// NewClient is used to initialize a client. See ClientConf for the available options.
func NewClient(conf ClientConf) (*Client, error) {
	rand.Seed(time.Now().UnixNano())

	if conf.IsPassive == false && (conf.TcpPort == 0 || conf.UdpPort == 0) {
		return nil, fmt.Errorf("tcp and udp ports must be both set when in active mode")
	}
	if conf.IsPassive == false && conf.PeerEncryptionMode != ForceEncryption && conf.TcpPort == 0 {
		return nil, fmt.Errorf("tcp port must be set when in active mode and encryption is optional")
	}
	if conf.IsPassive == false && conf.PeerEncryptionMode != DisableEncryption && conf.TcpTlsPort == 0 {
		return nil, fmt.Errorf("tcp tls port must be set when in active mode and encryption is on")
	}
	if conf.TcpPort != 0 && conf.TcpPort == conf.TcpTlsPort {
		return nil, fmt.Errorf("tcp port and tcp tls port cannot be the same")
	}
	if conf.DownloadMaxParallel == 0 {
		conf.DownloadMaxParallel = 6
	}
	if conf.UploadMaxParallel == 0 {
		conf.UploadMaxParallel = 10
	}
	if conf.HubConnTries == 0 {
		conf.HubConnTries = 3
	}
	if conf.Nick == "" {
		return nil, fmt.Errorf("nick is mandatory")
	}
	if conf.UploadMaxSpeed == 0 {
		conf.UploadMaxSpeed = 2 * 1024 * 1024
	}
	if conf.ClientString == "" {
		conf.ClientString = "++" // verified
	}
	if conf.ClientVersion == "" {
		conf.ClientVersion = "0.868" // verified
	}
	if conf.PkValue == "" {
		conf.PkValue = "DCPLUSPLUS0.868" // verified
	}
	if conf.ListGenerator == "" {
		conf.ListGenerator = "DC++ 0.868" // verified
	}

	u, err := url.Parse(conf.HubUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to parse hub url")
	}
	if _, ok := map[string]struct{}{
		"adc":   {},
		"adcs":  {},
		"dchub": {},
		"nmdc":  {},
		"nmdcs": {},
	}[u.Scheme]; !ok {
		return nil, fmt.Errorf("unsupported protocol: %s", u.Scheme)
	}
	if u.Port() == "" {
		if u.Scheme == "adc" {
			u.Host = u.Hostname() + ":5000"
		} else if u.Scheme == "adcs" {
			u.Host = u.Hostname() + ":5001"
		} else {
			u.Host = u.Hostname() + ":411"
		}
	}
	conf.HubUrl = u.String()

	c := &Client{
		conf:                  conf,
		privateId:             conf.PID,
		terminate:             make(chan struct{}),
		proto:                 protocolNMDC,
		hubIsEncrypted:        u.Scheme == "adcs" || u.Scheme == "nmdcs",
		hubHostname:           u.Hostname(),
		hubPort:               atoui(u.Port()),
		shareRoots:            make(map[string]string),
		shareTree:             make(map[string]*shareDirectory),
		peers:                 make(map[string]*Peer),
		downloadSlotAvail:     conf.DownloadMaxParallel,
		uploadSlotAvail:       conf.UploadMaxParallel,
		peerConns:             make(map[*peerConn]struct{}),
		peerConnsByKey:        make(map[nickDirectionPair]*peerConn),
		transfers:             make(map[transfer]struct{}),
		activeDownloadsByPeer: make(map[string]*Download),
	}
	if u.Scheme == "adc" || u.Scheme == "adcs" {
		c.proto = protocolADC
	}

	// generate privateId if not provided (random)
	var zeroPID atypes.PID
	if c.privateId == zeroPID {
		c.privateId, _ = atypes.NewPID()
	}

	// generate clientId (hash of privateId)
	hasher := tiger.NewHash()
	hasher.Write(c.privateId[:])
	hasher.Sum(c.clientId[:0])

	if err := newHubConn(c); err != nil {
		return nil, err
	}

	if err := newshareIndexer(c); err != nil {
		return nil, err
	}

	if c.conf.IsPassive == false && c.conf.PeerEncryptionMode != ForceEncryption {
		if err := newListenerTcp(c, false); err != nil {
			return nil, err
		}
	}

	if c.conf.IsPassive == false && c.conf.PeerEncryptionMode != DisableEncryption {
		if err := newListenerTcp(c, true); err != nil {
			return nil, err
		}
	}

	if c.conf.IsPassive == false {
		if err := newListenerUdp(c); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Client) getProto() protocolName {
	return protocolName(atomic.LoadUint32((*uint32)(&c.proto)))
}

func (c *Client) setProto(p protocolName) {
	atomic.StoreUint32((*uint32)(&c.proto), uint32(p))
}

func (c *Client) protoIsAdc() bool {
	return c.getProto() == protocolADC
}

// Close every open connection and stop the client.
func (c *Client) Close() error {
	if c.terminateRequested == true {
		return nil
	}
	c.terminateRequested = true
	close(c.terminate)
	return nil
}

// Run starts the client and waits until the client has been terminated.
func (c *Client) Run() {
	// get an ip
	if c.conf.IsPassive == false {
		if c.conf.Ip != "" {
			c.ip = c.conf.Ip
		} else {
			if err := c.getPublicIp(); err != nil {
				panic(err)
			}
		}
	}

	c.wg.Add(1)
	go c.shareIndexer.do()

	if c.listenerTcp != nil {
		c.wg.Add(1)
		go c.listenerTcp.do()
	}
	if c.tcpTlsListener != nil {
		c.wg.Add(1)
		go c.tcpTlsListener.do()
	}
	if c.listenerUdp != nil {
		c.wg.Add(1)
		go c.listenerUdp.do()
	}

	if c.OnInitialized != nil {
		c.OnInitialized()
	}

	c.Safe(func() {
		if c.conf.HubManualConnect == false {
			c.HubConnect()
		}
	})

	<-c.terminate

	c.Safe(func() {
		c.hubConn.close()
		for t := range c.transfers {
			t.Close()
		}
		for p := range c.peerConns {
			p.close()
		}
		if c.listenerUdp != nil {
			c.listenerUdp.close()
		}
		if c.tcpTlsListener != nil {
			c.tcpTlsListener.close()
		}
		if c.listenerTcp != nil {
			c.listenerTcp.close()
		}
		c.shareIndexer.close()
	})

	c.wg.Wait()
}

func (c *Client) getPublicIp() error {
	res, err := http.Get(_PUBLIC_IP_PROVIDER)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return err
	}

	m := rePublicIp.FindStringSubmatch(string(body))
	if m == nil {
		return fmt.Errorf("cannot obtain ip")
	}

	c.ip = m[1]
	return nil
}

func (c *Client) sendInfos(firstTime bool) {
	hubUnregisteredCount := uint(0)
	hubRegisteredCount := uint(0)
	hubOperatorCount := uint(0)

	if c.hubConn.passwordSent == true {
		hubRegisteredCount = 1
	} else {
		hubUnregisteredCount = 1
	}

	if c.protoIsAdc() {
		info := &adc.UserInfo{
			Desc:           c.conf.Description,
			ShareFiles:     int(c.shareCount),
			ShareSize:      int64(c.shareSize),
			HubsNormal:     int(hubUnregisteredCount),
			HubsRegistered: int(hubRegisteredCount),
			HubsOperator:   int(hubOperatorCount),
			Application:    c.conf.ClientString,  // verified
			Version:        c.conf.ClientVersion, // verified
			MaxUpload:      numtoa(c.conf.UploadMaxSpeed),
			Slots:          int(c.conf.UploadMaxParallel),
		}

		info.Features = append(info.Features, adc.FeaADC0)
		if c.conf.IsPassive == false {
			info.Features = append(info.Features, adc.FeaTCP4, adc.FeaUDP4)
		}
		if c.conf.PeerEncryptionMode != DisableEncryption {
			info.Features = append(info.Features, adc.FeaADCS)
		}

		if c.conf.IsPassive == false {
			info.Ip4 = c.ip
			info.Udp4 = int(c.conf.UdpPort)
		}

		// these must be sent only during initialization
		if firstTime == true {
			info.Name = c.conf.Nick
			info.Id = c.clientId
			info.Pid = &c.privateId

			if c.conf.PeerEncryptionMode != DisableEncryption &&
				c.conf.IsPassive == false {
				info.KP = c.adcFingerprint
			}
		}

		c.hubConn.conn.Write(&proto.AdcBInfos{
			&adc.BroadcastPacket{ID: c.adcSessionId},
			info,
		})

	} else {
		// http://nmdc.sourceforge.net/Versions/NMDC-1.3.html#_myinfo
		// https://web.archive.org/web/20150323115608/http://wiki.gusari.org/index.php?title=$MyINFO
		userFlag := nmdc.FlagStatusNormal

		// add upload and download TLS support
		if c.conf.PeerEncryptionMode != DisableEncryption {
			userFlag |= nmdc.FlagTLSDownload | nmdc.FlagTLSUpload
		}

		c.hubConn.conn.Write(&nmdc.MyINFO{
			Name: c.conf.Nick,
			Desc: c.conf.Description,
			Client: types.Software{
				Name:    c.conf.ClientString,
				Version: c.conf.ClientVersion,
			},
			Mode: func() nmdc.UserMode {
				if c.conf.IsPassive == false {
					return nmdc.UserModeActive
				}
				return nmdc.UserModePassive
			}(),
			HubsNormal:     int(hubUnregisteredCount),
			HubsRegistered: int(hubRegisteredCount),
			HubsOperator:   int(hubOperatorCount),
			Slots:          int(c.conf.UploadMaxParallel),
			Conn:           fmt.Sprintf("%d KiB/s", c.conf.UploadMaxSpeed/1024),
			Flag:           userFlag,
			Email:          c.conf.Email,
			ShareSize:      c.shareSize,
		})
	}
}

// Safe is used to safely execute code outside the client context. It must be
// used when interacting with the client outside the callbacks (i.e. inside a
// parallel goroutine).
func (c *Client) Safe(cb func()) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	cb()
}

// Conf returns the configuration passed during client initialization.
func (c *Client) Conf() ClientConf {
	return c.conf
}
