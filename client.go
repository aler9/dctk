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
    "strings"
    "regexp"
    "io/ioutil"
    "net/http"
    "math/rand"
    "net/url"
)

const _PUBLIC_IP_PROVIDER = "http://checkip.dyndns.org/"
var rePublicIp = regexp.MustCompile("("+reStrIp+")")

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
    IsPassive                   bool
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
    // an email, optional
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

    // options useful only for debugging purposes
    PeerDisableCompression      bool
    HubDisableKeepAlive         bool
}

type Client struct {
    conf                    ClientConf
    state                   string
    mutex                   sync.Mutex
    wg                      sync.WaitGroup
    wakeUp                  chan struct{}
    protoIsAdc              bool
    hubIsEncrypted          bool
    hubHostname             string
    hubPort                 uint
    hubSolvedIp             string
    ip                      string
    shareIndexer            *shareIndexer
    shareRoots              map[string]string
    shareTree               map[string]*shareDirectory
    shareCount              uint
    shareSize               uint64
    fileList                []byte
    listenerTcp             *listenerTcp
    tcpTlsListener          *listenerTcp
    listenerUdp             *listenerUdp
    connHub                 *connHub
    // we follow the ADC way to handle IDs, even when using NMDC
    privateId               []byte
    clientId                []byte
    sessionId               string // we save it encoded since it is 20 bits and cannot be decoded easily
    peers                   map[string]*Peer
    downloadSlotAvail       uint
    uploadSlotAvail         uint
    connPeers               map[*connPeer]struct{}
    connPeersByKey          map[nickDirectionPair]*connPeer
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
    OnMessagePublic         func(p *Peer, content string)
    // called when a private message has been received
    OnMessagePrivate        func(p *Peer, content string)
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
    if conf.Connection == "" {
        conf.Connection = "Cable"
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
        conf: conf,
        state: "running",
        wakeUp: make(chan struct{}, 1),
        protoIsAdc: (u.Scheme == "adc" || u.Scheme == "adcs"),
        hubIsEncrypted: (u.Scheme == "adcs" || u.Scheme == "nmdcs"),
        hubHostname: u.Hostname(),
        hubPort: atoui(u.Port()),
        shareRoots: make(map[string]string),
        shareTree: make(map[string]*shareDirectory),
        peers: make(map[string]*Peer),
        downloadSlotAvail: conf.DownloadMaxParallel,
        uploadSlotAvail: conf.UploadMaxParallel,
        connPeers: make(map[*connPeer]struct{}),
        connPeersByKey: make(map[nickDirectionPair]*connPeer),
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

    if err := newConnHub(c); err != nil {
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
    if c.conf.IsPassive == false {
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

    <- c.wakeUp

    c.Safe(func() {
        c.connHub.terminate()
        for t,_ := range c.transfers {
            t.terminate()
        }
        for p,_ := range c.connPeers {
            p.terminate()
        }
        if c.listenerUdp != nil {
            c.listenerUdp.terminate()
        }
        if c.tcpTlsListener != nil {
            c.tcpTlsListener.terminate()
        }
        if c.listenerTcp != nil {
            c.listenerTcp.terminate()
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

func (c *Client) sendInfos(firstTime bool) {
    if c.protoIsAdc == true {
        supports := []string{ "SEGA", "ADC0" }
        //if c.PeerEncryptionMode != DisableEncryption {
        //    supports = append(supports, "ADCS")
        //}
        if c.conf.IsPassive == false {
            supports = append(supports, "TCP4", "UDP4")
        }

        fields := map[string]string{
            adcFieldDescription: c.conf.Description,
            adcFieldShareCount: numtoa(c.shareCount),
            adcFieldShareSize: numtoa(c.shareSize),
            adcFieldHubUnregisteredCount: numtoa(c.conf.HubUnregisteredCount),
            adcFieldHubRegisteredCount: numtoa(c.conf.HubRegisteredCount),
            adcFieldHubOperatorCount: numtoa(c.conf.HubOperatorCount),
            adcFieldSoftware: c.conf.ClientString, // verified
            adcFieldVersion: c.conf.ClientVersion, // verified
            adcFieldSupports: strings.Join(supports, ","),
            adcFieldUploadSpeed: "655",
            adcFieldUploadSlotCount: numtoa(c.conf.UploadMaxParallel),
        }

        // these must be send only during initialization
        if firstTime == true {
            fields[adcFieldName] = c.conf.Nick
            fields[adcFieldClientId] = dcBase32Encode(c.clientId)
            fields[adcFieldPrivateId] = dcBase32Encode(c.privateId)
        }

        if c.conf.IsPassive == false {
            fields[adcFieldIp] = c.ip
            fields[adcFieldUdpPort] = numtoa(c.conf.UdpPort)
        }

        c.connHub.conn.Write(&msgAdcBInfos{
            msgAdcTypeB{ SessionId: c.sessionId },
            msgAdcKeyInfos{ Fields: fields },
        })

    } else {
        modestr := "P"
        if c.conf.IsPassive == false {
            modestr = "A"
        }

        // http://nmdc.sourceforge.net/Versions/NMDC-1.3.html#_myinfo
        // https://web.archive.org/web/20150323115608/http://wiki.gusari.org/index.php?title=$MyINFO
        var statusByte byte = 0x01

        // add upload and download TLS support
        if c.conf.PeerEncryptionMode != DisableEncryption {
            statusByte |= (0x01 << 4) | (0x01 << 5)
        }

        c.connHub.conn.Write(&msgNmdcMyInfo{
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
