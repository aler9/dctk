package dctoolkit

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

const (
	// client <-> hub features
	nmdcFeatureUserCommands = "UserCommand"
	nmdcFeatureNoGetInfo    = "NoGetINFO"
	nmdcFeatureNoHello      = "NoHello"
	nmdcFeatureUserIp       = "UserIP2"
	nmdcFeatureTTHSearch    = "TTHSearch"
	nmdcFeatureZlibFull     = "ZPipe0"
	nmdcFeatureTls          = "TLS"
	// client <-> client features
	nmdcFeatureMiniSlots    = "MiniSlots"
	nmdcFeatureFileListBzip = "XmlBZList"
	nmdcFeatureAdcGet       = "ADCGet"
	nmdcFeatureTTHLeaves    = "TTHL"
	nmdcFeatureTTHDownload  = "TTHF"
	nmdcFeatureZlibGet      = "ZLIG"
)

var reNmdcCommand = regexp.MustCompile("(?s)^\\$([a-zA-Z0-9]+)( (.+))?$")
var reNmdcPublicChat = regexp.MustCompile("(?s)^<(" + reStrNick + "|.+?)> (.+)$") // some very bad hubs also use spaces in public message authors
var reNmdcPrivateChat = regexp.MustCompile("(?s)^\\$To: (" + reStrNick + ") From: (" + reStrNick + ") \\$<(" + reStrNick + ")> (.+)$")

var reNmdcCmdConnectToMe = regexp.MustCompile("^(" + reStrNick + ") (" + reStrIp + "):(" + reStrPort + ")(S?)$")
var reNmdcCmdDirection = regexp.MustCompile("^(Download|Upload) ([0-9]+)$")
var reNmdcCmdForceMove = regexp.MustCompile("^(" + reStrAddress + ")(:(" + reStrPort + "))?$")
var reNmdcCmdInfo = regexp.MustCompile("^\\$ALL (" + reStrNick + ") (.*?)(<(.*?) V:(.+?),M:(A|P),H:([0-9]+)/([0-9]+)/([0-9]+),S:([0-9]+)>)?\\$ \\$(.*?)(.)\\$(.*?)\\$([0-9]+)\\$$")
var reNmdcCmdLock = regexp.MustCompile("^([^ ]+)( Pk=(.+?)(Ref=(.+?))?)?$")
var reNmdcCmdRevConnectToMe = regexp.MustCompile("^(" + reStrNick + ") (" + reStrNick + ")$")
var reNmdcCmdSearchReqActive = regexp.MustCompile("^(" + reStrIp + "):(" + reStrPort + ") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")
var reNmdcCmdSearchReqPassive = regexp.MustCompile("^Hub:(" + reStrNick + ") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")
var reNmdcCmdSearchResult = regexp.MustCompile("^(" + reStrNick + ") ([^\x05]+?)(\x05([0-9]+))? ([0-9]+)/([0-9]+)\x05TTH:(" + reStrTTH + ") \\((" + reStrIp + "):(" + reStrPort + ")\\)$")
var reNmdcCmdUserCommand = regexp.MustCompile("^([0-9]{1,3}) ([0-9]{1,2}) (.*?)$")
var reNmdcCmdUserIP = regexp.MustCompile("^(" + reStrNick + ") (" + reStrIp + ")$")

// http://nmdc.sourceforge.net/Versions/NMDC-1.3.html#_key
// https://web.archive.org/web/20150529002427/http://wiki.gusari.org/index.php?title=LockToKey%28%29
func nmdcComputeKey(lock []byte) []byte {
	// the key has exactly as many characters as the lock
	key := make([]byte, len(lock))

	// Except for the first, each key character is computed from the corresponding lock character and the one before it
	key[0] = 0
	for n := 1; n < len(key); n++ {
		key[n] = lock[n] ^ lock[n-1]
	}

	// The first key character is calculated from the first lock character and the last two lock characters
	key[0] = lock[0] ^ lock[len(lock)-1] ^ lock[len(lock)-2] ^ 5

	// Next, every character in the key must be nibble-swapped
	for n := 0; n < len(key); n++ {
		key[n] = ((key[n] << 4) & 240) | ((key[n] >> 4) & 15)
	}

	// the characters with the decimal ASCII values of 0, 5, 36, 96, 124, and 126
	// cannot be sent to the server. Each character with this value must be
	// substituted with the string /%DCN000%/, /%DCN005%/, /%DCN036%/, /%DCN096%/, /%DCN124%/, or /%DCN126%/
	var res []byte
	for _, byt := range key {
		if byt == 0 || byt == 5 || byt == 36 || byt == 96 || byt == 124 || byt == 126 {
			res = append(res, []byte(fmt.Sprintf("/%%DCN%.3d%%/", byt))...)
		} else {
			res = append(res, byt)
		}
	}
	return res
}

func nmdcCommandEncode(key string, args string) string {
	return "$" + key + " " + args + "|"
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

func (p *protocolNmdc) Read() (msgDecodable, error) {
	if p.readBinary == false {
		msgStr, err := p.ReadMessage()
		if err != nil {
			return nil, err
		}

		msg, err := func() (msgDecodable, error) {
			if len(msgStr) == 0 {
				return &msgNmdcKeepAlive{}, nil

			} else if matches := reNmdcCommand.FindStringSubmatch(msgStr); matches != nil {
				key, args := matches[1], matches[3]

				cmd := func() msgNmdcCommandDecodable {
					switch key {
					case "ADCGET":
						return &msgNmdcGetFile{}
					case "ADCSND":
						return &msgNmdcSendFile{}
					case "BadPass":
						return &msgNmdcBadPassword{}
					case "BotList":
						return &msgNmdcBotList{}
					case "ConnectToMe":
						return &msgNmdcConnectToMe{}
					case "Direction":
						return &msgNmdcDirection{}
					case "Error":
						return &msgNmdcError{}
					case "ForceMove":
						return &msgNmdcForceMove{}
					case "GetPass":
						return &msgNmdcGetPass{}
					case "Hello":
						return &msgNmdcHello{}
					case "HubName":
						return &msgNmdcHubName{}
					case "HubIsFull":
						return &msgNmdcHubIsFull{}
					case "HubTopic":
						return &msgNmdcHubTopic{}
					case "Key":
						return &msgNmdcKey{}
					case "Lock":
						return &msgNmdcLock{}
					case "LogedIn":
						return &msgNmdcLoggedIn{}
					case "MaxedOut":
						return &msgNmdcMaxedOut{}
					case "MyINFO":
						return &msgNmdcMyInfo{}
					case "MyNick":
						return &msgNmdcMyNick{}
					case "OpList":
						return &msgNmdcOpList{}
					case "Quit":
						return &msgNmdcQuit{}
					case "RevConnectToMe":
						return &msgNmdcRevConnectToMe{}
					case "Search":
						return &msgNmdcSearchRequest{}
					case "SR":
						return &msgNmdcSearchResult{}
					case "Supports":
						return &msgNmdcSupports{}
					case "UserCommand":
						return &msgNmdcUserCommand{}
					case "UserIP":
						return &msgNmdcUserIp{}
					case "ValidateDenide":
						return &msgNmdcValidateDenide{}
					case "ZOn":
						return &msgNmdcZon{}
					}
					return nil
				}()
				if cmd == nil {
					return nil, fmt.Errorf("unrecognized command")
				}

				err := cmd.NmdcDecode(args)
				if err != nil {
					return nil, fmt.Errorf("unable to decode arguments")
				}
				return cmd, nil

			} else if matches := reNmdcPublicChat.FindStringSubmatch(msgStr); matches != nil {
				return &msgNmdcPublicChat{Author: matches[1], Content: matches[2]}, nil

			} else if matches := reNmdcPrivateChat.FindStringSubmatch(msgStr); matches != nil {
				return &msgNmdcPrivateChat{Author: matches[3], Content: matches[4]}, nil

			} else {
				return nil, fmt.Errorf("unknown sequence")
			}
		}()
		if err != nil {
			return nil, fmt.Errorf("Unable to parse: %s (%s)", err, msgStr)
		}

		dolog(LevelDebug, "[%s->c] %T %+v", p.remoteLabel, msg, msg)
		return msg, nil

	} else {
		buf, err := p.ReadBinary()
		if err != nil {
			return nil, err
		}
		return &msgBinary{buf}, nil
	}
}

func (p *protocolNmdc) Write(msg msgEncodable) {
	nmdc, ok := msg.(msgNmdcEncodable)
	if !ok {
		panic(fmt.Errorf("command not fit for nmdc (%T)", msg))
	}
	dolog(LevelDebug, "[c->%s] %T %+v", p.remoteLabel, msg, msg)
	p.protocolBase.Write([]byte(nmdc.NmdcEncode()))
}

type msgNmdcCommandDecodable interface {
	NmdcDecode(args string) error
}

type msgNmdcEncodable interface {
	NmdcEncode() string
}

type msgNmdcGetFile struct {
	Query      string
	Start      uint64
	Length     int64
	Compressed bool
}

func (m *msgNmdcGetFile) NmdcDecode(args string) error {
	matches := reSharedCmdAdcGet.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Query, m.Start, m.Length, m.Compressed = matches[1], atoui64(matches[4]),
		atoi64(matches[5]), (matches[6] != "")
	return nil
}

func (m *msgNmdcGetFile) NmdcEncode() string {
	return nmdcCommandEncode("ADCGET", fmt.Sprintf("%s %d %d%s",
		m.Query, m.Start, m.Length,
		func() string {
			if m.Compressed == true {
				return " ZL1"
			}
			return ""
		}()))
}

type msgNmdcSendFile struct {
	Query      string
	Start      uint64
	Length     uint64
	Compressed bool
}

func (m *msgNmdcSendFile) NmdcDecode(args string) error {
	matches := reSharedCmdAdcSnd.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Query, m.Start, m.Length, m.Compressed = matches[1], atoui64(matches[4]),
		atoui64(matches[5]), (matches[6] != "")
	return nil
}

func (m *msgNmdcSendFile) NmdcEncode() string {
	return nmdcCommandEncode("ADCSND", fmt.Sprintf("%s %d %d%s",
		m.Query, m.Start, m.Length,
		func() string {
			if m.Compressed {
				return " ZL1"
			}
			return ""
		}()))
}

type msgNmdcBadPassword struct{}

func (m *msgNmdcBadPassword) NmdcDecode(args string) error {
	return nil
}

type msgNmdcBotList struct {
	Bots map[string]struct{}
}

func (m *msgNmdcBotList) NmdcDecode(args string) error {
	m.Bots = make(map[string]struct{})
	for _, bot := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
		m.Bots[bot] = struct{}{}
	}
	return nil
}

type msgNmdcConnectToMe struct {
	Target    string
	Ip        string
	Port      uint
	Encrypted bool
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

func (m *msgNmdcConnectToMe) NmdcEncode() string {
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
	Direction string
	Bet       uint
}

func (m *msgNmdcDirection) NmdcDecode(args string) error {
	matches := reNmdcCmdDirection.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Direction, m.Bet = matches[1], atoui(matches[2])
	return nil
}

func (m *msgNmdcDirection) NmdcEncode() string {
	return nmdcCommandEncode("Direction", fmt.Sprintf("%s %d", m.Direction, m.Bet))
}

type msgNmdcError struct {
	Error string
}

func (m *msgNmdcError) NmdcDecode(args string) error {
	m.Error = args
	return nil
}

func (m *msgNmdcError) NmdcEncode() string {
	return nmdcCommandEncode("Error", m.Error)
}

type msgNmdcForceMove struct {
	Address string
	Port    uint
}

func (m *msgNmdcForceMove) NmdcDecode(args string) error {
	matches := reNmdcCmdForceMove.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Address, m.Port = matches[1], atoui(matches[3])
	return nil
}

type msgNmdcGetNickList struct{}

func (m *msgNmdcGetNickList) NmdcEncode() string {
	return nmdcCommandEncode("GetNickList", "")
}

type msgNmdcGetPass struct{}

func (m *msgNmdcGetPass) NmdcDecode(args string) error {
	return nil
}

type msgNmdcKeepAlive struct{}

func (m *msgNmdcKeepAlive) NmdcEncode() string {
	return "|"
}

type msgNmdcHello struct{}

func (m *msgNmdcHello) NmdcDecode(args string) error {
	return nil
}

type msgNmdcHubIsFull struct{}

func (m *msgNmdcHubIsFull) NmdcDecode(args string) error {
	return nil
}

type msgNmdcHubName struct {
	Content string
}

func (m *msgNmdcHubName) NmdcDecode(args string) error {
	m.Content = args
	return nil
}

type msgNmdcHubTopic struct {
	Content string
}

func (m *msgNmdcHubTopic) NmdcDecode(args string) error {
	m.Content = args
	return nil
}

type msgNmdcKey struct {
	Key []byte
}

func (m *msgNmdcKey) NmdcDecode(args string) error {
	m.Key = []byte(args)
	return nil
}

func (m *msgNmdcKey) NmdcEncode() string {
	return nmdcCommandEncode("Key", string(m.Key))
}

type msgNmdcLock struct {
	Lock string
	Pk   string
	Ref  string
}

func (m *msgNmdcLock) NmdcDecode(args string) error {
	matches := reNmdcCmdLock.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Lock, m.Pk, m.Ref = matches[1], matches[3], matches[5]
	return nil
}

func (m *msgNmdcLock) NmdcEncode() string {
	ret := m.Lock
	if m.Pk != "" {
		ret += " Pk=" + m.Pk
		if m.Ref != "" {
			ret += "Ref=" + m.Ref
		}
	}
	return nmdcCommandEncode("Lock", ret)
}

type msgNmdcLoggedIn struct{}

func (m *msgNmdcLoggedIn) NmdcDecode(args string) error {
	return nil
}

type msgNmdcMaxedOut struct{}

func (m *msgNmdcMaxedOut) NmdcDecode(args string) error {
	return nil
}

func (m *msgNmdcMaxedOut) NmdcEncode() string {
	return nmdcCommandEncode("MaxedOut", "")
}

type msgNmdcMyInfo struct {
	Nick                 string
	Description          string
	Client               string
	Version              string
	Mode                 string
	HubUnregisteredCount uint
	HubRegisteredCount   uint
	HubOperatorCount     uint
	UploadSlots          uint
	Connection           string
	StatusByte           byte
	Email                string
	ShareSize            uint64
}

func (m *msgNmdcMyInfo) NmdcDecode(args string) error {
	matches := reNmdcCmdInfo.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Nick, m.Description, m.Client, m.Version, m.Mode, m.HubUnregisteredCount,
		m.HubRegisteredCount, m.HubOperatorCount, m.UploadSlots, m.Connection,
		m.StatusByte, m.Email, m.ShareSize = matches[1], matches[2], matches[4],
		matches[5], matches[6], atoui(matches[7]), atoui(matches[8]),
		atoui(matches[9]), atoui(matches[10]), matches[11],
		[]byte(matches[12])[0], matches[13], atoui64(matches[14])
	return nil
}

func (m *msgNmdcMyInfo) NmdcEncode() string {
	return nmdcCommandEncode("MyINFO", fmt.Sprintf(
		"$ALL %s %s<%s V:%s,M:%s,H:%d/%d/%d,S:%d>$ $%s%s$%s$%d$",
		m.Nick, m.Description, m.Client, m.Version, m.Mode,
		m.HubUnregisteredCount, m.HubRegisteredCount, m.HubOperatorCount,
		m.UploadSlots, m.Connection,
		string([]byte{m.StatusByte}), m.Email, m.ShareSize))
}

type msgNmdcMyNick struct {
	Nick string
}

func (m *msgNmdcMyNick) NmdcDecode(args string) error {
	m.Nick = args
	return nil
}

func (m *msgNmdcMyNick) NmdcEncode() string {
	return nmdcCommandEncode("MyNick", m.Nick)
}

type msgNmdcMyPass struct {
	Pass string
}

func (m *msgNmdcMyPass) NmdcEncode() string {
	return nmdcCommandEncode("MyPass", m.Pass)
}

type msgNmdcOpList struct {
	Ops map[string]struct{}
}

func (m *msgNmdcOpList) NmdcDecode(args string) error {
	m.Ops = make(map[string]struct{})
	for _, op := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
		m.Ops[op] = struct{}{}
	}
	return nil
}

type msgNmdcPrivateChat struct {
	Author  string
	Dest    string
	Content string
}

func (c *msgNmdcPrivateChat) NmdcEncode() string {
	return fmt.Sprintf("$To: %s From: %s $<%s> %s|", c.Dest, c.Author, c.Author, c.Content)
}

type msgNmdcPublicChat struct {
	Author  string
	Content string
}

func (c *msgNmdcPublicChat) NmdcEncode() string {
	return fmt.Sprintf("<%s> %s|", c.Author, c.Content)
}

type msgNmdcQuit struct {
	Nick string
}

func (m *msgNmdcQuit) NmdcDecode(args string) error {
	m.Nick = args
	return nil
}

type msgNmdcRevConnectToMe struct {
	Author string
	Target string
}

func (m *msgNmdcRevConnectToMe) NmdcDecode(args string) error {
	matches := reNmdcCmdRevConnectToMe.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Author, m.Target = matches[1], matches[2]
	return nil
}

func (m *msgNmdcRevConnectToMe) NmdcEncode() string {
	return nmdcCommandEncode("RevConnectToMe", fmt.Sprintf("%s %s", m.Author, m.Target))
}

type msgNmdcSearchRequest struct {
	Type     nmdcSearchType
	MinSize  uint64
	MaxSize  uint64
	Query    string
	IsActive bool
	Ip       string // active only
	UdpPort  uint   // active only
	Nick     string // passive only
}

func (m *msgNmdcSearchRequest) NmdcDecode(args string) error {
	if matches := reNmdcCmdSearchReqActive.FindStringSubmatch(args); matches != nil {
		m.IsActive = true
		m.Ip, m.UdpPort = matches[1], atoui(matches[2])
		m.MaxSize = func() uint64 {
			if matches[3] == "T" && matches[4] == "T" {
				return atoui64(matches[5])
			}
			return 0
		}()
		m.MinSize = func() uint64 {
			if matches[3] == "T" && matches[4] == "F" {
				return atoui64(matches[5])
			}
			return 0
		}()
		m.Type = nmdcSearchType(atoi(matches[6]))
		m.Query = nmdcSearchUnescape(matches[7])

	} else if matches := reNmdcCmdSearchReqPassive.FindStringSubmatch(args); matches != nil {
		m.IsActive = false
		m.Nick = matches[1]
		m.MaxSize = func() uint64 {
			if matches[2] == "T" && matches[3] == "T" {
				return atoui64(matches[4])
			}
			return 0
		}()
		m.MinSize = func() uint64 {
			if matches[2] == "T" && matches[3] == "F" {
				return atoui64(matches[4])
			}
			return 0
		}()
		m.Type = nmdcSearchType(atoi(matches[5]))
		m.Query = nmdcSearchUnescape(matches[6])

	} else {
		return fmt.Errorf("invalid args")
	}
	return nil
}

func (m *msgNmdcSearchRequest) NmdcEncode() string {
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
		func() uint64 {
			if m.MaxSize != 0 {
				return m.MaxSize
			}
			return m.MinSize
		}(),
		m.Type,
		func() string {
			if m.Type == nmdcSearchTypeTTH {
				return "TTH:" + m.Query
			}
			return nmdcSearchEscape(m.Query)
		}()))
}

type msgNmdcSearchResult struct {
	Path       string
	IsDir      bool
	Size       uint64 // file only, also directory in ADC
	TTH        string // file only
	Nick       string
	SlotAvail  uint
	SlotCount  uint
	HubIp      string
	HubPort    uint
	TargetNick string // send only, passive only
}

func (m *msgNmdcSearchResult) NmdcDecode(args string) error {
	matches := reNmdcCmdSearchResult.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	m.Nick, m.Path, m.Size, m.SlotAvail, m.SlotCount, m.TTH, m.IsDir, m.HubIp,
		m.HubPort = matches[1], "/"+strings.Replace(matches[2], "\\", "/", -1),
		func() uint64 {
			if matches[3] != "" {
				return atoui64(matches[4])
			}
			return 0
		}(),
		atoui(matches[5]), atoui(matches[6]),
		func() string {
			if matches[3] != "" {
				return matches[7]
			}
			return ""
		}(),
		(matches[3] == ""),
		matches[8], atoui(matches[9])
	return nil
}

func (m *msgNmdcSearchResult) NmdcEncode() string {
	return nmdcCommandEncode("SR", fmt.Sprintf("%s %s%s %d/%d\x05TTH:%s (%s:%d)%s",
		m.Nick,
		strings.Replace(m.Path[1:], "/", "\\", -1), // skip first slash
		func() string {
			if m.IsDir == false {
				return fmt.Sprintf("\x05%d", m.Size)
			}
			return ""
		}(),
		m.SlotAvail, m.SlotCount,
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
	Features map[string]struct{}
}

func (m *msgNmdcSupports) NmdcDecode(args string) error {
	m.Features = make(map[string]struct{})
	for _, feat := range strings.Split(args, " ") {
		m.Features[feat] = struct{}{}
	}
	return nil
}

func (m *msgNmdcSupports) NmdcEncode() string {
	var ret []string
	for feat := range m.Features {
		ret = append(ret, feat)
	}
	return nmdcCommandEncode("Supports", strings.Join(ret, " "))
}

type msgNmdcUserCommand struct{}

func (m *msgNmdcUserCommand) NmdcDecode(args string) error {
	matches := reNmdcCmdUserCommand.FindStringSubmatch(args)
	if matches == nil {
		return errorArgsFormat
	}
	return nil
}

type msgNmdcUserIp struct {
	Ips map[string]string
}

func (m *msgNmdcUserIp) NmdcDecode(args string) error {
	m.Ips = make(map[string]string)
	for _, ipstr := range strings.Split(strings.TrimSuffix(args, "$$"), "$$") {
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

func (m *msgNmdcValidateNick) NmdcEncode() string {
	return nmdcCommandEncode("ValidateNick", m.Nick)
}

type msgNmdcVersion struct{}

func (m *msgNmdcVersion) NmdcEncode() string {
	return nmdcCommandEncode("Version", "1,0091")
}

type msgNmdcValidateDenide struct{}

func (m *msgNmdcValidateDenide) NmdcDecode(args string) error {
	return nil
}

type msgNmdcZon struct{}

func (m *msgNmdcZon) NmdcDecode(args string) error {
	return nil
}
