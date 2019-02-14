// dctoolkit is a library that implements the client part of the Direct Connect
// peer-to-peer system (ADC and NMDC protocols) in the Go programming language.
// It allows the creation of clients that can interact with hubs and other
// clients, and can be used as backend to user interfaces or automatic bots.
package dctoolkit

import (
    "sync"
    "fmt"
    "net"
    "time"
    "regexp"
    "io/ioutil"
    "net/http"
    "math/rand"
    "net/url"
)

const _PUBLIC_IP_PROVIDER = "http://checkip.dyndns.org/"
var rePublicIp = regexp.MustCompile("("+reStrIp+")")

type Peer struct {
    // peer nickname
    Nick            string
    // peer description, if provided
    Description     string
    // peer email, if provided
    Email           string
    // total size of files shared by peer
    ShareSize       uint64
    // peer ip, if sent by hub
    Ip              string
    // whether peer is a bot
    IsBot           bool
    // whether peer is a operator
    IsOperator      bool
    // (adc only) peer session id
    AdcSessionId    string
    // (adc only) peer client id
    AdcClientId     []byte
    // (adc only) peer supported features
    AdcSupports     []string
    // (nmdc only) peer connection string
    NmdcConnection  string
    // (nmdc only) peer status byte
    NmdcStatusByte  byte
}

func (p *Peer) supportTls() bool {
    // we check only for bit 4
    return (p.NmdcStatusByte & (0x01 << 4)) == (0x01 << 4)
}

type transfer interface {
    isTransfer()
    terminate()
}

type EncryptionMode int
const (
    // use encryption when the two peers both support it
    PreferEncryption EncryptionMode = iota
    // disable competely encryption
    DisableEncryption
    // do not interact with peers that do not support encrypton
    ForceEncryption
)

type ClientConf struct {
    // turns on passive mode: it is not necessary anymore to open TcpPort, UdpPort
    // and TcpTlsPort on your router but functionalities are limited
    ModePassive                 bool
    // whether to use the local IP instead of the IP of your internet provider
    PrivateIp                   bool
    // these are the 3 ports needed for active mode. They must be accessible from the
    // internet, so your router must be configured
    TcpPort                     uint
    UdpPort                     uint
    TcpTlsPort                  uint
    // the maximum number of file to download in parallel. When this number is
    // exceeded, the other downloads are queued and started when a slot becomes available
    DownloadMaxParallel         uint
    // the maximum number of file to upload in parallel
    UploadMaxParallel           uint
    // disables compression. Can be useful for debug purposes
    PeerDisableCompression      bool
    // set the policy regarding encryption with other peers. See EncryptionMode for options
    PeerEncryptionMode          EncryptionMode
    // The hub url in the format protocol://address:port
    // supported protocols are adc, adcs, nmdc and nmdcs
    HubUrl                      string
    // how many times attempting connection with hub before giving up
    HubConnTries                uint
    // disables compression. Can be useful for debug purposes
    HubDisableCompression       bool
    // if turned on, connection to hub is not automatic and HubConnect() must be
    // called manually
    HubManualConnect            bool
    // the nickname to use in the hub and with other peers
    Nick                        string
    // the password associated with the nick, if requested by the hub
    Password                    string
    // an email, optionall
    Email                       string
    // a description, optional
    Description                 string
    // the connection string, it influences the icon other peers see
    Connection                  string
    // these are used to identify your client. By default they mimic the DC++ ones
    ClientString                string
    ClientVersion               string
    PkValue                     string
    ListGenerator               string
    // these are variables sent to the hub, in this library they are static
    HubUnregisteredCount        uint
    HubRegisteredCount          uint
    HubOperatorCount            uint
}

type Client struct {
    conf                    ClientConf
    state                   string
    mutex                   sync.Mutex
    wg                      sync.WaitGroup
    wakeUp                  chan struct{}
    hubIsAdc                bool
    hubIsEncrypted          bool
    hubHostname             string
    hubPort                 uint
    hubSolvedIp             string
    ip                      string
    shareIndexer            *shareIndexer
    shareRoots              map[string]string
    shareTree               map[string]*shareRootDirectory
    shareCount              uint
    shareSize               uint64
    fileList                []byte
    tcpListener             *tcpListener
    tcpTlsListener          *tcpListener
    udpListener             *udpListener
    hubConn                 *hubConn
    // we follow the ADC way to handle IDs, even when using NMDC
    privateId               []byte
    clientId                []byte
    sessionId               string // we save it encoded since it is 20 bits and cannot be decoded easily
    peers                   map[string]*Peer
    downloadSlotAvail       uint
    uploadSlotAvail         uint
    peerConns               map[*peerConn]struct{}
    peerConnsByKey          map[nickDirectionPair]*peerConn
    transfers               map[transfer]struct{}
    activeDownloadsByPeer   map[string]*Download

    // called just after client initialization, before connecting to the hub
    OnInitialized           func()
    // called every time the share indexer has finished indexing the client share
    OnShareIndexed          func()
    // called when the connection between client and hub has been established
    OnHubConnected          func()
    // called when a critical error happens
    OnHubError              func(err error)
    // called when a peer connects to the hub
    OnPeerConnected         func(p *Peer)
    // called when a peer has just updated its informations
    OnPeerUpdated           func(p *Peer)
    // called when a peer disconnects from the hub
    OnPeerDisconnected      func(p *Peer)
    // called when someone has written in the hub public chat
    OnPublicMessage         func(p *Peer, content string)
    // called when a private message has been received
    OnPrivateMessage        func(p *Peer, content string)
    // called when a seearch result has been received
    OnSearchResult          func(r *SearchResult)
    // called when a given download has finished
    OnDownloadSuccessful    func(d *Download)
    // called when a given download has failed
    OnDownloadError         func(d *Download)
}

// NewClient is used to initialize a client. See ClientConf for the available options.
func NewClient(conf ClientConf) (*Client,error) {
    rand.Seed(time.Now().UnixNano())

    if conf.ModePassive == false && (conf.TcpPort == 0 || conf.UdpPort == 0) {
        return nil, fmt.Errorf("tcp and udp ports must be both set when in active mode")
    }
    if conf.ModePassive == false && conf.PeerEncryptionMode != ForceEncryption && conf.TcpPort == 0 {
        return nil, fmt.Errorf("tcp port must be set when in active mode and encryption is optional")
    }
    if conf.ModePassive == false && conf.PeerEncryptionMode != DisableEncryption && conf.TcpTlsPort == 0 {
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
    if conf.Connection == "" {
        conf.Connection = "Cable"
    }
    if conf.ClientString == "" {
        conf.ClientString = "++" // ok
    }
    if conf.ClientVersion == "" {
        conf.ClientVersion = "0.868" // ok
    }
    if conf.PkValue == "" {
        conf.PkValue = "DCPLUSPLUS0.868" // ok
    }
    if conf.ListGenerator == "" {
        conf.ListGenerator = "DC++ 0.868" // ok
    }
    if conf.HubRegisteredCount == 0 {
        conf.HubRegisteredCount = 1
    }

    u,err := url.Parse(conf.HubUrl)
    if err != nil {
        return nil, fmt.Errorf("unable to parse hub url")
    }
    if _,ok := map[string]struct{}{
        "adc": struct{}{},
        "adcs": struct{}{},
        "nmdc": struct{}{},
        "nmdcs": struct{}{},
    }[u.Scheme]; !ok {
        return nil, fmt.Errorf("unsupported protocol: %s", u.Scheme)
    }
    if u.Port() == "" {
        u.Host = u.Hostname() + ":411"
    }
    conf.HubUrl = u.String()

    c := &Client{
        conf: conf,
        state: "running",
        wakeUp: make(chan struct{}, 1),
        hubIsAdc: (u.Scheme == "adc" || u.Scheme == "adcs"),
        hubIsEncrypted: (u.Scheme == "adcs" || u.Scheme == "nmdcs"),
        hubHostname: u.Hostname(),
        hubPort: atoui(u.Port()),
        shareRoots: make(map[string]string),
        shareTree: make(map[string]*shareRootDirectory),
        peers: make(map[string]*Peer),
        downloadSlotAvail: conf.DownloadMaxParallel,
        uploadSlotAvail: conf.UploadMaxParallel,
        peerConns: make(map[*peerConn]struct{}),
        peerConnsByKey: make(map[nickDirectionPair]*peerConn),
        transfers: make(map[transfer]struct{}),
        activeDownloadsByPeer: make(map[string]*Download),
    }

    // generate privateId (random)
    c.privateId = make([]byte, 24)
    rand.Read(c.privateId)

    // generate clientId (hash of privateId)
    hasher := tigerNew()
    hasher.Write(c.privateId)
    c.clientId = hasher.Sum(nil)

    if err := newshareIndexer(c); err != nil {
        return nil, err
    }

    if c.conf.ModePassive == false && c.conf.PeerEncryptionMode != ForceEncryption {
        if err := newTcpListener(c, false); err != nil {
            return nil, err
        }
    }

    if c.conf.ModePassive == false && c.conf.PeerEncryptionMode != DisableEncryption {
        if err := newTcpListener(c, true); err != nil {
            return nil, err
        }
    }

    if c.conf.ModePassive == false {
        if err := newUdpListener(c); err != nil {
            return nil, err
        }
    }

    if err := newHub(c); err != nil {
        return nil, err
    }

    return c, nil
}

// Terminate close every open connection and stop the client.
func (c *Client) Terminate() {
    switch c.state {
    case "terminated":
        return
    }
    c.state = "terminated"
    dolog(LevelInfo, "[terminating]")
    c.wakeUp <- struct{}{}
}

// Run starts the client and waits until the client has been terminated.
func (c *Client) Run() {
    // get an ip
    if c.conf.ModePassive == false {
        if c.conf.PrivateIp == false {
            if err := c.dlPublicIp(); err != nil {
                panic(err)
            }
        } else {
            if err := c.getPrivateIp(); err != nil {
                panic(err)
            }
        }
    }

    c.wg.Add(1)
    go c.shareIndexer.do()

    if c.tcpListener != nil {
        c.wg.Add(1)
        go c.tcpListener.do()
    }
    if c.tcpTlsListener != nil {
        c.wg.Add(1)
        go c.tcpTlsListener.do()
    }
    if c.udpListener != nil {
        c.wg.Add(1)
        go c.udpListener.do()
    }

    if c.OnInitialized != nil {
        c.OnInitialized()
    }

    c.Safe(func() {
        if c.conf.HubManualConnect == false {
            c.HubConnect()
        }
    })

    <- c.wakeUp

    c.Safe(func() {
        c.hubConn.terminate()
        for t,_ := range c.transfers {
            t.terminate()
        }
        for p,_ := range c.peerConns {
            p.terminate()
        }
        if c.udpListener != nil {
            c.udpListener.terminate()
        }
        if c.tcpTlsListener != nil {
            c.tcpTlsListener.terminate()
        }
        if c.tcpListener != nil {
            c.tcpListener.terminate()
        }
        c.shareIndexer.terminate()
    })

    c.wg.Wait()
}

func (c *Client) dlPublicIp() error {
    res,err := http.Get(_PUBLIC_IP_PROVIDER)
    if err != nil {
        return err
    }

    body,err := ioutil.ReadAll(res.Body)
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

func (c *Client) getPrivateIp() error {
    addrs, err := net.InterfaceAddrs()
    if err != nil {
        return err
    }

    for _,a := range addrs {
        if ipnet,ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
                c.ip = ipnet.IP.String()
                break
            }
        }
    }
    if c.ip == "" {
        return fmt.Errorf("cannot find own ip")
    }
    return nil
}

func (c *Client) myInfo() {
    modestr := "P"
    if c.conf.ModePassive == false {
        modestr = "A"
    }

    // http://nmdc.sourceforge.net/Versions/NMDC-1.3.html#_myinfo
    // https://web.archive.org/web/20150323115608/http://wiki.gusari.org/index.php?title=$MyINFO
    var statusByte byte = 0x01

    // add upload and download TLS support
    if c.conf.PeerEncryptionMode != DisableEncryption {
        statusByte |= (0x01 << 4) | (0x01 << 5)
    }

    c.hubConn.conn.Send(&msgNmdcMyInfo{
        Nick: c.conf.Nick,
        Description: c.conf.Description,
        Client: c.conf.ClientString,
        Version: c.conf.ClientVersion,
        Mode: modestr,
        HubUnregisteredCount: c.conf.HubUnregisteredCount,
        HubRegisteredCount: c.conf.HubRegisteredCount,
        HubOperatorCount: c.conf.HubOperatorCount,
        UploadSlots: c.conf.UploadMaxParallel,
        Connection: c.conf.Connection,
        StatusByte: statusByte,
        Email: c.conf.Email,
        ShareSize: c.shareSize,
    })
}

func (c *Client) connectToMe(target string) {
    p,ok := c.peers[target]
    if !ok {
        return
    }

    c.hubConn.conn.Send(&msgNmdcConnectToMe{
        Target: target,
        Ip: c.ip,
        Port: func() uint {
            if c.conf.PeerEncryptionMode != DisableEncryption && p.supportTls() {
                return c.conf.TcpTlsPort
            }
            return c.conf.TcpPort
        }(),
        Encrypted: (c.conf.PeerEncryptionMode != DisableEncryption && p.supportTls()),
    })
}

func (c *Client) revConnectToMe(target string) {
    c.hubConn.conn.Send(&msgNmdcRevConnectToMe{
        Author: c.conf.Nick,
        Target: target,
    })
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

// DownloadCount returns the number of remaining downloads, queued or active.
func (c *Client) DownloadCount() int {
    count := 0
    for t,_ := range c.transfers {
        if _,ok := t.(*Download); ok {
            count++
        }
    }
    return count
}

// Peers returns a map containing all the peers connected to current hub.
func (c *Client) Peers() map[string]*Peer {
    return c.peers
}

// PublicMessage publishes a message in the hub public chat.
func (c *Client) PublicMessage(content string) {
    if c.hubIsAdc == true {
        c.hubConn.conn.Send(&msgAdcBMessage{
            msgAdcTypeB{ c.sessionId },
            msgAdcKeyMessage{ Content: content },
        })

    } else {
        c.hubConn.conn.Send(&msgNmdcPublicChat{ c.conf.Nick, content })
    }
}

// PrivateMessage sends a private message to a specific peer connected to the hub.
func (c *Client) PrivateMessage(dest *Peer, content string) {
    if c.hubIsAdc == true {
        c.hubConn.conn.Send(&msgAdcDMessage{
            msgAdcTypeD{ c.sessionId, dest.AdcSessionId },
            msgAdcKeyMessage{ Content: content },
        })

    } else {
        c.hubConn.conn.Send(&msgNmdcPrivateChat{ c.conf.Nick, dest.Nick, content })
    }
}
