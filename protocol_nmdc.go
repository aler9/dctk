package dctoolkit

import (
    "fmt"
    "strings"
    "regexp"
    "net"
)

const dirTTH = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var reNmdcCmdAdcGet = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) (-1|[0-9]+)( ZL1)?$")
var reAdcCmdSnd = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) ([0-9]+)( ZL1)?$")
var reNmdcCmdConnectToMe = regexp.MustCompile("^("+reStrNick+") ("+reStrIp+"):("+reStrPort+")(S?)$")
var reNmdcCmdDirection = regexp.MustCompile("^(Download|Upload) ([0-9]+)$")
var reNmdcCmdInfo = regexp.MustCompile("^\\$ALL ("+reStrNick+") (.*?) ?\\$ \\$(.*?)(.)\\$(.*?)\\$([0-9]+)\\$$")
var reNmdcCmdRevConnectToMe = regexp.MustCompile("^("+reStrNick+") ("+reStrNick+")$")
var reNmdcCmdSearchReqActive = regexp.MustCompile("^("+reStrIp+"):("+reStrPort+") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")
var reNmdcCmdSearchReqPassive = regexp.MustCompile("^Hub:("+reStrNick+") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")
var reNmdcCmdSearchResult = regexp.MustCompile("^("+reStrNick+") (.+?) ([0-9]+)/([0-9]+)\x05TTH:("+reStrTTH+") \\(("+reStrIp+"):("+reStrPort+")\\)$")
var reNmdcCmdUserCommand = regexp.MustCompile("^([0-9]+) ([0-9]{1,2}) (.*?)$")
var reNmdcCmdUserIP = regexp.MustCompile("^("+reStrNick+") ("+reStrIp+")$")

func nmdcCommandEncode(key string, args string) []byte {
    return []byte(fmt.Sprintf("$%s %s|", key, args))
}

type protocolNmdc struct {
    *protocolBase
}

func newProtocolNmdc(remoteLabel string, nconn net.Conn,
    applyReadTimeout bool, applyWriteTimeout bool) protocol {
    p := &protocolNmdc{
        protocolBase: newProtocolBase(remoteLabel,
            nconn, applyReadTimeout, applyWriteTimeout, '|'),
    }
    return p
}

func (p *protocolNmdc) Read() (msgDecodable,error) {
    if p.readBinary == false {
        for {
            buf,err := p.ReadMessage()
            if err != nil {
                return nil,err
            }

            msgStr := string(buf)
            var msg msgDecodable

            if len(msgStr) == 0 { // empty message: skip
                continue

            } else if matches := reNmdcCommand.FindStringSubmatch(msgStr); matches != nil {
                key, args := matches[1], matches[3]

                cmd := func() msgNmdcCommandDecodable {
                    switch key {
                    case "ADCGET": return &msgNmdcAdcGet{}
                    case "ADCSND": return &msgNmdcAdcSnd{}
                    case "BotList": return &msgNmdcBotList{}
                    case "ConnectToMe": return &msgNmdcConnectToMe{}
                    case "Direction": return &msgNmdcDirection{}
                    case "Error": return &msgNmdcError{}
                    case "ForceMove": return &msgNmdcForceMove{}
                    case "GetPass": return &msgNmdcGetPass{}
                    case "Hello": return &msgNmdcHello{}
                    case "HubName": return &msgNmdcHubName{}
                    case "HubTopic": return &msgNmdcHubTopic{}
                    case "Key": return &msgNmdcKey{}
                    case "Lock": return &msgNmdcLock{}
                    case "LogedIn": return &msgNmdcLoggedIn{}
                    case "MaxedOut": return &msgNmdcMaxedOut{}
                    case "MyINFO": return &msgNmdcMyInfo{}
                    case "MyNick": return &msgNmdcMyNick{}
                    case "OpList": return &msgNmdcOpList{}
                    case "Quit": return &msgNmdcQuit{}
                    case "RevConnectToMe": return &msgNmdcRevConnectToMe{}
                    case "Search": return &msgNmdcSearchRequest{}
                    case "SR": return &msgNmdcSearchResult{}
                    case "Supports": return &msgNmdcSupports{}
                    case "UserCommand": return &msgNmdcUserCommand{}
                    case "UserIP": return &msgNmdcUserIp{}
                    case "ZOn": return &msgNmdcZon{}
                    }
                    return nil
                }()
                if cmd == nil {
                    return nil, fmt.Errorf("unrecognized command: %s", msgStr)
                }

                err := cmd.NmdcDecode(args)
                if err != nil {
                    return nil, fmt.Errorf("unable to decode arguments for %s: %s", key, err)
                }
                msg = cmd

            } else if matches := reNmdcPublicChat.FindStringSubmatch(msgStr); matches != nil {
                msg = &msgNmdcPublicChat{ Author: matches[1], Content: matches[2] }

            } else if matches := reNmdcPrivateChat.FindStringSubmatch(msgStr); matches != nil {
                msg = &msgNmdcPrivateChat{ Author: matches[3], Content: matches[4] }

            } else {
                return nil, fmt.Errorf("Unable to parse: %s", msgStr)
            }

            dolog(LevelDebug, "[%s->c] %T %+v", p.remoteLabel, msg, msg)
            return msg, nil
        }
    } else {
        buf,err := p.ReadBinary()
        if err != nil {
            return nil, err
        }
        return &msgNmdcBinary{ Content: buf }, nil
    }
}

func (c *protocolNmdc) Write(msg msgEncodable) {
    nmdc,ok := msg.(msgNmdcEncodable)
    if !ok {
        panic("command not fit for nmdc")
    }
    dolog(LevelDebug, "[c->%s] %T %+v", c.remoteLabel, msg, msg)
    c.sendChan <- nmdc.NmdcEncode()
}

type msgNmdcCommandDecodable interface {
    NmdcDecode(args string) error
}

type msgNmdcEncodable interface {
    NmdcEncode()    []byte
}

type msgNmdcAdcGet struct {
    Query           string
    Start           uint64
    Length          int64
    Compress        bool
}

func (m *msgNmdcAdcGet) NmdcDecode(args string) error {
    matches := reNmdcCmdAdcGet.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Query, m.Start, m.Length, m.Compress = matches[1], atoui64(matches[4]),
        atoi64(matches[5]), (matches[6] != "")
    return nil
}

func (m *msgNmdcAdcGet) NmdcEncode() []byte {
    return nmdcCommandEncode("ADCGET", fmt.Sprintf("%s %d %d%s",
        m.Query, m.Start, m.Length,
        func() string {
            if m.Compress == true {
                return " ZL1"
            }
            return ""
        }()))
}

type msgNmdcAdcSnd struct {
    Query       string
    Start       uint64
    Length      uint64
    Compressed  bool
}

func (m *msgNmdcAdcSnd) NmdcDecode(args string) error {
    matches := reAdcCmdSnd.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Query, m.Start, m.Length, m.Compressed = matches[1], atoui64(matches[4]),
        atoui64(matches[5]), (matches[6] != "")
    return nil
}

func (m *msgNmdcAdcSnd) NmdcEncode() []byte {
    return nmdcCommandEncode("ADCSND", fmt.Sprintf("%s %d %d%s",
        m.Query, m.Start, m.Length,
        func() string {
            if m.Compressed {
                return " ZL1"
            }
            return ""
        }()))
}

type msgNmdcBinary struct {
    Content []byte
}

func (c *msgNmdcBinary) NmdcEncode() []byte {
    return c.Content
}

type msgNmdcBotList struct {
    Bots []string
}

func (m *msgNmdcBotList) NmdcDecode(args string) error {
    for _,bot := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
        m.Bots = append(m.Bots, bot)
    }
    return nil
}

type msgNmdcConnectToMe struct {
    Target      string
    Ip          string
    Port        uint
    Encrypted   bool
}

func (m *msgNmdcConnectToMe) NmdcDecode(args string) error {
    matches := reNmdcCmdConnectToMe.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Target, m.Ip, m.Port, m.Encrypted = matches[1], matches[2], atoui(matches[3]),
        (matches[4] != "")
    return nil
}

func (m *msgNmdcConnectToMe) NmdcEncode() []byte {
    return nmdcCommandEncode("ConnectToMe", fmt.Sprintf("%s %s:%d%s",
        m.Target, m.Ip, m.Port,
        func() string {
            if m.Encrypted {
                return "S"
            }
            return ""
        }()))
}

type msgNmdcDirection struct {
    Direction   string
    Bet         uint
}

func (m *msgNmdcDirection) NmdcDecode(args string) error {
    matches := reNmdcCmdDirection.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Direction, m.Bet = matches[1], atoui(matches[2])
    return nil
}

func (m *msgNmdcDirection) NmdcEncode() []byte {
    return nmdcCommandEncode("Direction", fmt.Sprintf("%s %d", m.Direction, m.Bet))
}

type msgNmdcError struct {
    Error string
}

func (m *msgNmdcError) NmdcDecode(args string) error {
    m.Error = args
    return nil
}

func (m *msgNmdcError) NmdcEncode() []byte {
    return nmdcCommandEncode("Error", m.Error)
}

type msgNmdcForceMove struct {}

func (m *msgNmdcForceMove) NmdcDecode(args string) error {
    return nil
}

type msgNmdcGetNickList struct {}

func (m *msgNmdcGetNickList) NmdcEncode() []byte {
    return nmdcCommandEncode("GetNickList", "")
}

type msgNmdcGetPass struct {}

func (m *msgNmdcGetPass) NmdcDecode(args string) error {
    return nil
}

type msgNmdcHello struct {}

func (m *msgNmdcHello) NmdcDecode(args string) error {
    return nil
}

type msgNmdcHubName struct {}

func (m *msgNmdcHubName) NmdcDecode(args string) error {
    return nil
}

type msgNmdcHubTopic struct {}

func (m *msgNmdcHubTopic) NmdcDecode(args string) error {
    return nil
}

type msgNmdcKey struct {
    Key []byte
}

func (m *msgNmdcKey) NmdcDecode(args string) error {
    m.Key = []byte(args)
    return nil
}

func (m *msgNmdcKey) NmdcEncode() []byte {
    return nmdcCommandEncode("Key", string(m.Key))
}

type msgNmdcLock struct {
    Values []string
}

func (m *msgNmdcLock) NmdcDecode(args string) error {
    m.Values = strings.Split(args, " ")
    return nil
}

func (m *msgNmdcLock) NmdcEncode() []byte {
    return nmdcCommandEncode("Lock", strings.Join(m.Values, " "))
}

type msgNmdcLoggedIn struct {}

func (m *msgNmdcLoggedIn) NmdcDecode(args string) error {
    return nil
}

type msgNmdcMaxedOut struct {}

func (m *msgNmdcMaxedOut) NmdcDecode(args string) error {
    return nil
}

func (m *msgNmdcMaxedOut) NmdcEncode() []byte {
    return nmdcCommandEncode("MaxedOut", "")
}

type msgNmdcMyInfo struct {
    Nick                    string
    Description             string
    Client                  string
    Version                 string
    Mode                    string
    HubUnregisteredCount    uint
    HubRegisteredCount      uint
    HubOperatorCount        uint
    UploadSlots             uint
    Connection              string
    StatusByte              byte
    Email                   string
    ShareSize               uint64
}

func (m *msgNmdcMyInfo) NmdcDecode(args string) error {
    matches := reNmdcCmdInfo.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Nick, m.Description, m.Connection, m.StatusByte, m.Email,
        m.ShareSize = matches[1], matches[2], matches[3], []byte(matches[4])[0],
        matches[5], atoui64(matches[6])
    return nil
}

func (m *msgNmdcMyInfo) NmdcEncode() []byte {
    return nmdcCommandEncode("MyINFO", fmt.Sprintf("$ALL %s %s <%s V:%s,M:%s,H:%d/%d/%d,S:%d>$ $%s%s$%s$%d$",
        m.Nick, m.Description, m.Client, m.Version, m.Mode,
        m.HubUnregisteredCount, m.HubRegisteredCount, m.HubOperatorCount,
        m.UploadSlots, m.Connection,
        string([]byte{ m.StatusByte }), m.Email, m.ShareSize))
}

type msgNmdcMyNick struct {
    Nick string
}

func (m *msgNmdcMyNick) NmdcDecode(args string) error {
    m.Nick = args
    return nil
}

func (m *msgNmdcMyNick) NmdcEncode() []byte {
    return nmdcCommandEncode("MyNick", m.Nick)
}

type msgNmdcMyPass struct {
    Pass string
}

func (m *msgNmdcMyPass) NmdcEncode() []byte {
    return nmdcCommandEncode("MyPass", m.Pass)
}

type msgNmdcOpList struct {
    Ops []string
}

func (m *msgNmdcOpList) NmdcDecode(args string) error {
    for _,op := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
        m.Ops = append(m.Ops, op)
    }
    return nil
}

type msgNmdcPrivateChat struct {
    Author      string
    Dest        string
    Content     string
}

func (c *msgNmdcPrivateChat) NmdcEncode() []byte {
    return []byte(fmt.Sprintf("$To: %s From: %s $<%s> %s|", c.Dest, c.Author, c.Author, c.Content))
}

type msgNmdcPublicChat struct {
    Author      string
    Content     string
}

func (c *msgNmdcPublicChat) NmdcEncode() []byte {
    return []byte(fmt.Sprintf("<%s> %s|", c.Author, c.Content))
}

type msgNmdcQuit struct {
    Nick string
}

func (m *msgNmdcQuit) NmdcDecode(args string) error {
    m.Nick = args
    return nil
}

type msgNmdcRevConnectToMe struct {
    Author      string
    Target      string
}

func (m *msgNmdcRevConnectToMe) NmdcDecode(args string) error {
    matches := reNmdcCmdRevConnectToMe.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Author, m.Target = matches[1], matches[2]
    return nil
}

func (m *msgNmdcRevConnectToMe) NmdcEncode() []byte {
    return nmdcCommandEncode("RevConnectToMe", fmt.Sprintf("%s %s", m.Author, m.Target))
}

type msgNmdcSearchRequest struct {
    Type        SearchType
    MinSize     uint
    MaxSize     uint
    Query       string
    IsActive    bool
    Ip          string  // active only
    UdpPort     uint    // active only
    Nick        string  // passive only
}

func (m *msgNmdcSearchRequest) NmdcDecode(args string) error {
    if matches := reNmdcCmdSearchReqActive.FindStringSubmatch(args); matches != nil {
        m.IsActive = true
        m.Ip, m.UdpPort = matches[1], atoui(matches[2])
        m.MaxSize = func() uint {
            if matches[3] == "T" && matches[4] == "T" {
                return atoui(matches[5])
            }
            return 0
        }()
        m.MinSize = func() uint {
            if matches[3] == "T" && matches[4] == "F" {
                return atoui(matches[5])
            }
            return 0
        }()
        m.Type = SearchType(atoi(matches[6]))
        m.Query = searchUnescape(matches[7])

    } else if matches := reNmdcCmdSearchReqPassive.FindStringSubmatch(args); matches != nil {
        m.IsActive = false
        m.Nick = matches[1]
        m.MaxSize = func() uint {
            if matches[2] == "T" && matches[3] == "T" {
                return atoui(matches[4])
            }
            return 0
        }()
        m.MinSize = func() uint {
            if matches[2] == "T" && matches[3] == "F" {
                return atoui(matches[4])
            }
            return 0
        }()
        m.Type = SearchType(atoi(matches[5]))
        m.Query = searchUnescape(matches[6])

    } else {
        return fmt.Errorf("invalid args")
    }
    return nil
}

func (m *msgNmdcSearchRequest) NmdcEncode() []byte {
    // <sizeRestricted>?<isMaxSize>?<size>?<fileType>?<searchPattern>
    return nmdcCommandEncode("Search", fmt.Sprintf("%s %s?%s?%d?%d?%s",
        func() string {
            if m.Ip != "" {
                return fmt.Sprintf("%s:%d", m.Ip, m.UdpPort)
            }
            return fmt.Sprintf("Hub:%s", m.Nick)
        }(),
        func() string {
            if m.MinSize != 0 || m.MaxSize != 0 {
                return "T"
            }
            return "F"
        }(),
        func() string {
            if m.MaxSize != 0 {
                return "T"
            }
            return "F"
        }(),
        func() uint {
            if m.MaxSize != 0 {
                return m.MaxSize
            }
            return m.MinSize
        }(),
        m.Type,
        func() string {
            if m.Type == TypeTTH {
                return "TTH:" + m.Query
            }
            return searchEscape(m.Query)
        }()))
}

type msgNmdcSearchResult struct {
    Nick            string
    Path            string
    SlotAvail       uint
    SlotCount       uint
    TTH             string
    IsDir           bool
    HubIp           string
    HubPort         uint
    TargetNick      string // send only, passive only
}

func (m *msgNmdcSearchResult) NmdcDecode(args string) error {
    matches := reNmdcCmdSearchResult.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Nick, m.Path, m.SlotAvail, m.SlotCount, m.TTH, m.IsDir, m.HubIp,
        m.HubPort = matches[1], matches[2], atoui(matches[3]), atoui(matches[4]),
        func() string {
            if matches[5] != dirTTH {
                return matches[5]
            }
            return ""
        }(),
        (matches[5] == dirTTH), matches[6], atoui(matches[7])
    return nil
}

func (m *msgNmdcSearchResult) NmdcEncode() []byte {
    return nmdcCommandEncode("SR", fmt.Sprintf("%s %s %d/%d\x05TTH:%s (%s:%d)%s",
        m.Nick, m.Path, m.SlotAvail, m.SlotCount,
        func() string {
            if m.IsDir == true {
                return dirTTH
            }
            return m.TTH
        }(),
        m.HubIp, m.HubPort,
        func() string {
            if m.TargetNick != "" {
                return "\x05" + m.TargetNick
            }
            return ""
        }()))
}

type msgNmdcSupports struct {
    Features []string
}

func (m *msgNmdcSupports) NmdcDecode(args string) error {
    m.Features = strings.Split(args, " ")
    return nil
}

func (m *msgNmdcSupports) NmdcEncode() []byte {
    return nmdcCommandEncode("Supports", strings.Join(m.Features, " "))
}

type msgNmdcUserCommand struct {}

func (m *msgNmdcUserCommand) NmdcDecode(args string) error {
    matches := reNmdcCmdUserCommand.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    return nil
}

type msgNmdcUserIp struct {
    Ips     map[string]string
}

func (m *msgNmdcUserIp) NmdcDecode(args string) error {
    m.Ips = make(map[string]string)
    for _,ipstr := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
        matches := reNmdcCmdUserIP.FindStringSubmatch(ipstr)
        if matches == nil {
            return errorArgsFormat
        }
        m.Ips[matches[1]] = matches[2]
    }
    return nil
}

type msgNmdcValidateNick struct {
    Nick string
}

func (m *msgNmdcValidateNick) NmdcEncode() []byte {
    return nmdcCommandEncode("ValidateNick", m.Nick)
}

type msgNmdcVersion struct {}

func (m *msgNmdcVersion) NmdcEncode() []byte {
    return nmdcCommandEncode("Version", "1,0091")
}

type msgNmdcZon struct {}

func (m *msgNmdcZon) NmdcDecode(args string) error {
    return nil
}
