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
)

const _PUBLIC_IP_PROVIDER = "http://checkip.dyndns.org/"
var rePublicIp = regexp.MustCompile("("+reStrIp+")")

type transfer interface {
    isTransfer()
    terminate()
}

type EncryptionMode int
const (
    PreferEncryption EncryptionMode = iota // default value
    DisableEncryption
    ForceEncryption
)

type ClientConf struct {
    ModePassive                 bool
    PrivateIp                   bool
    TcpPort                     uint
    UdpPort                     uint
    TcpTlsPort                  uint
    DownloadMaxParallel         uint
    UploadMaxParallel           uint
    PeerDisableCompression      bool
    PeerEncryptionMode          EncryptionMode
    HubAddress                  string
    HubPort                     uint
    HubConnTries                uint
    HubDisableCompression       bool
    HubManualConnect            bool
    Nick                        string
    Password                    string
    Email                       string
    Description                 string
    Connection                  string
    ClientString                string
    ClientVersion               string
    PkValue                     string
    ListGenerator               string
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
    clientId                string
    hubSolvedIp             string
    ip                      string
    shareIndexer            *shareIndexer
    shareRoots              map[string]string
    shareTree               map[string]*shareRootDirectory
    tthlDB                  map[string][]byte
    shareSize               uint64
    fileList                []byte
    tcpListener             *tcpListener
    tcpTlsListener          *tcpListener
    udpListener             *udpListener
    hubConn                 *hubConn
    downloadSlotAvail       uint
    uploadSlotAvail         uint
    peerConns               map[*peerConn]struct{}
    peerConnsByKey          map[nickDirectionPair]*peerConn
    transfers               map[transfer]struct{}
    activeDownloadsByPeer   map[string]*Download

    // hooks
    OnInitialized           func()
    OnShareIndexed          func()
    OnHubConnected          func()
    OnHubError              func(err error)
    OnPeerConnected         func(p *Peer)
    OnPeerUpdated           func(p *Peer)
    OnPeerDisconnected      func(p *Peer)
    OnPublicMessage         func(p *Peer, content string)
    OnPrivateMessage        func(p *Peer, content string)
    OnSearchResult          func(r *SearchResult)
    OnDownloadSuccessful    func(d *Download)
    OnDownloadError         func(d *Download)
}

func NewClient(conf ClientConf) (*Client,error) {
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
    if conf.HubAddress == "" {
        return nil, fmt.Errorf("hub ip is mandatory")
    }
    if conf.HubPort == 0 {
        conf.HubPort = 411
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

    rand.Seed(time.Now().UnixNano())

    c := &Client{
        conf: conf,
        state: "running",
        wakeUp: make(chan struct{}, 1),
        shareRoots: make(map[string]string),
        shareTree: make(map[string]*shareRootDirectory),
        tthlDB: make(map[string][]byte),
        downloadSlotAvail: conf.DownloadMaxParallel,
        uploadSlotAvail: conf.UploadMaxParallel,
        peerConns: make(map[*peerConn]struct{}),
        peerConnsByKey: make(map[nickDirectionPair]*peerConn),
        transfers: make(map[transfer]struct{}),
        activeDownloadsByPeer: make(map[string]*Download),
        clientId: dcRandomClientId(),
    }

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

func (c *Client) Terminate() {
    switch c.state {
    case "terminated":
        return
    }
    c.state = "terminated"
    dolog(LevelInfo, "[terminating]")
    c.wakeUp <- struct{}{}
}

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
    var status byte = 0x01

    // add upload and download TLS support
    if c.conf.PeerEncryptionMode != DisableEncryption {
        status |= (0x01 << 4) | (0x01 << 5)
    }

    c.hubConn.conn.Send <- msgCommand{ "MyINFO",
        fmt.Sprintf("$ALL %s %s <%s V:%s,M:%s,H:%d/%d/%d,S:%d>$ $%s%s$%s$%d$",
        c.conf.Nick, c.conf.Description, c.conf.ClientString, c.conf.ClientVersion, modestr,
        c.conf.HubUnregisteredCount, c.conf.HubRegisteredCount, c.conf.HubOperatorCount,
        c.conf.UploadMaxParallel, c.conf.Connection,
        string([]byte{status}), c.conf.Email, c.shareSize),
    }
}

func (c *Client) connectToMe(target string) {
    p,ok := c.hubConn.peers[target]
    if !ok {
        return
    }

    c.hubConn.conn.Send <- msgCommand{ "ConnectToMe",
        fmt.Sprintf("%s %s:%s",
            target,
            c.ip,
            func() string {
                if c.conf.PeerEncryptionMode != DisableEncryption && p.supportTls() {
                    return fmt.Sprintf("%dS", c.conf.TcpTlsPort)
                }
                return fmt.Sprintf("%d", c.conf.TcpPort)
            }()),
        }
}

func (c *Client) revConnectToMe(target string) {
    c.hubConn.conn.Send <- msgCommand{ "RevConnectToMe",
        fmt.Sprintf("%s %s", c.conf.Nick, target),
    }
}

func (c *Client) Safe(cb func()) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    cb()
}

func (c *Client) Conf() ClientConf {
    return c.conf
}

func (c *Client) DownloadCount() int {
    count := 0
    for t,_ := range c.transfers {
        if _,ok := t.(*Download); ok {
            count++
        }
    }
    return count
}

func (c *Client) Peers() map[string]*Peer {
    return c.hubConn.peers
}

func (c *Client) PublicMessage(content string) {
    c.hubConn.conn.Send <- msgPublicChat{ c.conf.Nick, content }
}

func (c *Client) PrivateMessage(dest *Peer, content string) {
    c.hubConn.conn.Send <- msgPrivateChat{ c.conf.Nick, dest.Nick, content }
}
