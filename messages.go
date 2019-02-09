package dctoolkit

import (
    "fmt"
    "strings"
    "regexp"
)

var errorArgsFormat = fmt.Errorf("not formatted correctly")

var reNmdcCommand = regexp.MustCompile("^\\$([a-zA-Z0-9]+)( ([^|]+))?\\|$")
var reNmdcPublicChat = regexp.MustCompile("^<("+reStrNick+")> ([^|]+)\\|$")
var reNmdcPrivateChat = regexp.MustCompile("^\\$To: ("+reStrNick+") From: ("+reStrNick+") \\$<("+reStrNick+")> ([^|]+)|$")

func dcCommandEncode(key string, args string) []byte {
    return []byte(fmt.Sprintf("$%s %s|", key, args))
}

type msgDecodable interface{}

type msgEncodable interface {
    Encode()     []byte
}

type msgDecodableNmdcCommand interface {
    DecodeArgs(args string) error
}

type msgNmdcAdcGet struct {
    Query       string
    Start       uint64
    Length      int64
    Compress    bool
}

var reCmdAdcGet = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) (-1|[0-9]+)( ZL1)?$")

func (m *msgNmdcAdcGet) DecodeArgs(args string) error {
    matches := reCmdAdcGet.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Query, m.Start, m.Length, m.Compress = matches[1], atoui64(matches[4]),
        atoi64(matches[5]), (matches[6] != "")
    return nil
}

func (m *msgNmdcAdcGet) Encode() []byte {
    return dcCommandEncode("ADCGET", fmt.Sprintf("%s %d %d%s",
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

var reCmdAdcSnd = regexp.MustCompile("^((file|tthl) TTH/("+reStrTTH+")|file files.xml.bz2) ([0-9]+) ([0-9]+)( ZL1)?$")

func (m *msgNmdcAdcSnd) DecodeArgs(args string) error {
    matches := reCmdAdcSnd.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Query, m.Start, m.Length, m.Compressed = matches[1], atoui64(matches[4]),
        atoui64(matches[5]), (matches[6] != "")
    return nil
}

func (m *msgNmdcAdcSnd) Encode() []byte {
    return dcCommandEncode("ADCSND", fmt.Sprintf("%s %d %d%s",
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

func (c *msgNmdcBinary) Encode() []byte {
    return c.Content
}

type msgNmdcBotList struct {
    Bots []string
}

func (m *msgNmdcBotList) DecodeArgs(args string) error {
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

var reCmdConnectToMe = regexp.MustCompile("^("+reStrNick+") ("+reStrIp+"):("+reStrPort+")(S?)$")

func (m *msgNmdcConnectToMe) DecodeArgs(args string) error {
    matches := reCmdConnectToMe.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Target, m.Ip, m.Port, m.Encrypted = matches[1], matches[2], atoui(matches[3]),
        (matches[4] != "")
    return nil
}

func (m *msgNmdcConnectToMe) Encode() []byte {
    return dcCommandEncode("ConnectToMe", fmt.Sprintf("%s %s:%d%s",
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

var reCmdDirection = regexp.MustCompile("^(Download|Upload) ([0-9]+)$")

func (m *msgNmdcDirection) DecodeArgs(args string) error {
    matches := reCmdDirection.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Direction, m.Bet = matches[1], atoui(matches[2])
    return nil
}

func (m *msgNmdcDirection) Encode() []byte {
    return dcCommandEncode("Direction", fmt.Sprintf("%s %d", m.Direction, m.Bet))
}

type msgNmdcError struct {
    Error string
}

func (m *msgNmdcError) DecodeArgs(args string) error {
    m.Error = args
    return nil
}

func (m *msgNmdcError) Encode() []byte {
    return dcCommandEncode("Error", m.Error)
}

type msgNmdcForceMove struct {}

func (m *msgNmdcForceMove) DecodeArgs(args string) error {
    return nil
}

type msgNmdcGetNickList struct {}

func (m *msgNmdcGetNickList) Encode() []byte {
    return dcCommandEncode("GetNickList", "")
}

type msgNmdcGetPass struct {}

func (m *msgNmdcGetPass) DecodeArgs(args string) error {
    return nil
}

type msgNmdcHello struct {}

func (m *msgNmdcHello) DecodeArgs(args string) error {
    return nil
}

type msgNmdcHubName struct {}

func (m *msgNmdcHubName) DecodeArgs(args string) error {
    return nil
}

type msgNmdcHubTopic struct {}

func (m *msgNmdcHubTopic) DecodeArgs(args string) error {
    return nil
}

type msgNmdcKey struct {
    Key string
}

func (m *msgNmdcKey) DecodeArgs(args string) error {
    m.Key = args
    return nil
}

func (m *msgNmdcKey) Encode() []byte {
    return dcCommandEncode("Key", m.Key)
}

type msgNmdcLock struct {
    Values []string
}

func (m *msgNmdcLock) DecodeArgs(args string) error {
    m.Values = strings.Split(args, " ")
    return nil
}

func (m *msgNmdcLock) Encode() []byte {
    return dcCommandEncode("Lock", strings.Join(m.Values, " "))
}

type msgNmdcLoggedIn struct {}

func (m *msgNmdcLoggedIn) DecodeArgs(args string) error {
    return nil
}

type msgNmdcMaxedOut struct {}

func (m *msgNmdcMaxedOut) DecodeArgs(args string) error {
    return nil
}

func (m *msgNmdcMaxedOut) Encode() []byte {
    return dcCommandEncode("MaxedOut", "")
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

var reCmdInfo = regexp.MustCompile("^\\$ALL ("+reStrNick+") (.*?) ?\\$ \\$(.*?)(.)\\$(.*?)\\$([0-9]+)\\$$")

func (m *msgNmdcMyInfo) DecodeArgs(args string) error {
    matches := reCmdInfo.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Nick, m.Description, m.Connection, m.StatusByte, m.Email,
        m.ShareSize = matches[1], matches[2], matches[3], []byte(matches[4])[0],
        matches[5], atoui64(matches[6])
    return nil
}

func (m *msgNmdcMyInfo) Encode() []byte {
    return dcCommandEncode("MyINFO", fmt.Sprintf("$ALL %s %s <%s V:%s,M:%s,H:%d/%d/%d,S:%d>$ $%s%s$%s$%d$",
        m.Nick, m.Description, m.Client, m.Version, m.Mode,
        m.HubUnregisteredCount, m.HubRegisteredCount, m.HubOperatorCount,
        m.UploadSlots, m.Connection,
        string([]byte{ m.StatusByte }), m.Email, m.ShareSize))
}

type msgNmdcMyNick struct {
    Nick string
}

func (m *msgNmdcMyNick) DecodeArgs(args string) error {
    m.Nick = args
    return nil
}

func (m *msgNmdcMyNick) Encode() []byte {
    return dcCommandEncode("MyNick", m.Nick)
}

type msgNmdcMyPass struct {
    Pass string
}

func (m *msgNmdcMyPass) Encode() []byte {
    return dcCommandEncode("MyPass", m.Pass)
}

type msgNmdcOpList struct {
    Ops []string
}

func (m *msgNmdcOpList) DecodeArgs(args string) error {
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

func (c *msgNmdcPrivateChat) Encode() []byte {
    return []byte(fmt.Sprintf("$To: %s From: %s $<%s> %s|", c.Dest, c.Author, c.Author, c.Content))
}

type msgNmdcPublicChat struct {
    Author      string
    Content     string
}

func (c *msgNmdcPublicChat) Encode() []byte {
    return []byte(fmt.Sprintf("<%s> %s|", c.Author, c.Content))
}

type msgNmdcQuit struct {
    Nick string
}

func (m *msgNmdcQuit) DecodeArgs(args string) error {
    m.Nick = args
    return nil
}

type msgNmdcRevConnectToMe struct {
    Author      string
    Target      string
}

var reCmdRevConnectToMe = regexp.MustCompile("^("+reStrNick+") ("+reStrNick+")$")

func (m *msgNmdcRevConnectToMe) DecodeArgs(args string) error {
    matches := reCmdRevConnectToMe.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Author, m.Target = matches[1], matches[2]
    return nil
}

func (m *msgNmdcRevConnectToMe) Encode() []byte {
    return dcCommandEncode("RevConnectToMe", fmt.Sprintf("%s %s", m.Author, m.Target))
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

var reCmdSearchReqActive = regexp.MustCompile("^("+reStrIp+"):("+reStrPort+") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")

var reCmdSearchReqPassive = regexp.MustCompile("^Hub:("+reStrNick+") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")

func (m *msgNmdcSearchRequest) DecodeArgs(args string) error {
    if matches := reCmdSearchReqActive.FindStringSubmatch(args); matches != nil {
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

    } else if matches := reCmdSearchReqPassive.FindStringSubmatch(args); matches != nil {
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

func (m *msgNmdcSearchRequest) Encode() []byte {
    // <sizeRestricted>?<isMaxSize>?<size>?<fileType>?<searchPattern>
    return dcCommandEncode("Search", fmt.Sprintf("%s %s?%s?%d?%d?%s",
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

const dirTTH = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var reCmdSearchResult = regexp.MustCompile("^("+reStrNick+") (.+?) ([0-9]+)/([0-9]+)\x05TTH:("+reStrTTH+") \\(("+reStrIp+"):("+reStrPort+")\\)$")

func (m *msgNmdcSearchResult) DecodeArgs(args string) error {
    matches := reCmdSearchResult.FindStringSubmatch(args)
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

func (m *msgNmdcSearchResult) Encode() []byte {
    return dcCommandEncode("SR", fmt.Sprintf("%s %s %d/%d\x05TTH:%s (%s:%d)%s",
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

func (m *msgNmdcSupports) DecodeArgs(args string) error {
    m.Features = strings.Split(args, " ")
    return nil
}

func (m *msgNmdcSupports) Encode() []byte {
    return dcCommandEncode("Supports", strings.Join(m.Features, " "))
}

type msgNmdcUserCommand struct {}

var reCmdUserCommand = regexp.MustCompile("^([0-9]+) ([0-9]{1,2}) (.*?)$")

func (m *msgNmdcUserCommand) DecodeArgs(args string) error {
    matches := reCmdUserCommand.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    return nil
}

type msgNmdcUserIp struct {
    Ips     map[string]string
}

var reCmdUserIP = regexp.MustCompile("^("+reStrNick+") ("+reStrIp+")$")

func (m *msgNmdcUserIp) DecodeArgs(args string) error {
    m.Ips = make(map[string]string)
    for _,ipstr := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
        matches := reCmdUserIP.FindStringSubmatch(ipstr)
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

func (m *msgNmdcValidateNick) Encode() []byte {
    return dcCommandEncode("ValidateNick", m.Nick)
}

type msgNmdcVersion struct {}

func (m *msgNmdcVersion) Encode() []byte {
    return dcCommandEncode("Version", "1,0091")
}

type msgNmdcZon struct {}

func (m *msgNmdcZon) DecodeArgs(args string) error {
    return nil
}
