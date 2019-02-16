package dctoolkit

import (
    "fmt"
    "strings"
    "regexp"
    "net"
)

const (
    // generic
    adcFieldCategory             = "CT"
    adcFieldDescription          = "DE"
    adcFieldEmail                = "EM"
    adcFieldClientId             = "ID"
    adcFieldName                 = "NI"
    adcFieldUploadSpeed          = "US"
    adcFieldUploadSlotCount      = "SL"
    adcFieldToken                = "TO"
    // client info
    adcFieldSoftware             = "AP"
    adcFieldVersion              = "VE"
    adcFieldHubUnregisteredCount = "HN"
    adcFieldHubRegisteredCount   = "HR"
    adcFieldHubOperatorCount     = "HO"
    adcFieldSupports             = "SU"
    adcFieldShareSize            = "SS"
    adcFieldShareCount           = "SF"
    adcFieldIp                   = "I4"
    adcFieldUdpPort              = "U4"
    adcFieldPrivateId            = "PD"
    // search requests & results
    adcFieldMinSize              = "GE"
    adcFieldMaxSize              = "LE"
    adcFieldIsFileOrFolder       = "TY"
    adcFieldFileExtension        = "EX"
    adcFieldQueryAnd             = "AN"
    adcFieldFileQueryOr          = "NO"
    adcFieldFilePath             = "FN"
    adcFieldFileSize             = "SI"
    adcFieldFileExactSize        = "EQ"
    adcFieldFileTTH              = "TR"
    adcFieldFileGroup            = "GR"
    adcFieldFileExcludeExtens    = "RX"
)

const reStrAdcSessionId = "[A-Z0-9]{4}"
const reStrAdcClientId = "[A-Z0-9]+"

var reAdcTypeB = regexp.MustCompile("^("+reStrAdcSessionId+") ")
var reAdcTypeD = regexp.MustCompile("^("+reStrAdcSessionId+") ("+reStrAdcSessionId+") ")
var reAdcTypeF = regexp.MustCompile("^("+reStrAdcSessionId+") (((\\+|-)[A-Za-z0-9]+)+) ")
var reAdcTypeU = regexp.MustCompile("^("+reStrAdcClientId+") ")

var reAdcGetPass = regexp.MustCompile("^[A-Z0-9]{3,}$")
var reAdcQuit = regexp.MustCompile("^("+reStrAdcSessionId+")( (.+))?$")
var reAdcConnectToMe = regexp.MustCompile("^(.+?) ("+reStrPort+") ([0-9]+)$")
var reAdcRevConnectToMe = regexp.MustCompile("^(.+?) ([0-9]+)$")
var readcSessionId = regexp.MustCompile("^"+reStrAdcSessionId+"$")
var reAdcStatus = regexp.MustCompile("^([0-9]+) (.+)$")

func adcUnescape(in string) string {
    in = strings.Replace(in, "\\s", " ", -1)
    in = strings.Replace(in, "\\n", "\n", -1)
    in = strings.Replace(in, "\\\\", "\\", -1)
    return in
}

func adcEscape(in string) string {
    in = strings.Replace(in, "\\", "\\\\", -1)
    in = strings.Replace(in, "\n", "\\n", -1)
    in = strings.Replace(in, " ", "\\s", -1)
    return in
}

func adcFieldsDecode(in string) map[string]string {
    ret := make(map[string]string)
    for _,arg := range strings.Split(in, " ") {
        if len(arg) < 2 {
            continue
        }
        ret[arg[:2]] = adcUnescape(arg[2:])
    }
    return ret
}

func adcFieldsEncode(fields map[string]string) string {
    var out []string
    for key,val := range fields {
        out = append(out, key + adcEscape(val))
    }
    return strings.Join(out, " ")
}

type protocolAdc struct {
    *protocolBase
}

func newProtocolAdc(remoteLabel string, nconn net.Conn,
    applyReadTimeout bool, applyWriteTimeout bool) protocol {
    p := &protocolAdc{
        protocolBase: newProtocolBase(remoteLabel,
            nconn, applyReadTimeout, applyWriteTimeout, '\n'),
    }
    return p
}

func (p *protocolAdc) Read() (msgDecodable,error) {
    if p.readBinary == false {
        msgStr,err := p.ReadMessage()
        if err != nil {
            return nil,err
        }

        msg,err := func() (msgDecodable,error) {
            if len(msgStr) < 5 {
                return nil, fmt.Errorf("message too short")
            }

            if msgStr[4] != ' ' {
                return nil, fmt.Errorf("invalid message")
            }

            msg := func() msgAdcTypeKeyDecodable {
                switch msgStr[:4] {
                case "BINF": return &msgAdcBInfos{}
                case "BMSG": return &msgAdcBMessage{}
                case "BSCH": return &msgAdcBSearchRequest{}
                case "CINF": return &msgAdcCInfos{}
                case "CSUP": return &msgAdcCSupports{}
                case "CSTA": return &msgAdcCStatus{}
                case "DCTM": return &msgAdcDConnectToMe{}
                case "DMSG": return &msgAdcDMessage{}
                case "DRCM": return &msgAdcDRevConnectToMe{}
                case "DRES": return &msgAdcDSearchResult{}
                case "FSCH": return &msgAdcFSearchRequest{}
                case "ICMD": return &msgAdcICommand{}
                case "IGPA": return &msgAdcIGetPass{}
                case "IINF": return &msgAdcIInfos{}
                case "IQUI": return &msgAdcIQuit{}
                case "ISID": return &msgAdcISessionId{}
                case "ISTA": return &msgAdcIStatus{}
                case "ISUP": return &msgAdcISupports{}
                }
                return nil
            }()
            if msg == nil {
                return nil, fmt.Errorf("unrecognized message")
            }

            n,err := msg.AdcTypeDecode(msgStr[5:])
            if err != nil {
                return nil, fmt.Errorf("unable to decode type")
            }

            err = msg.AdcKeyDecode(msgStr[5+n:])
            if err != nil {
                return nil, fmt.Errorf("unable to decode key")
            }

            return msg, nil
        }()
        if err != nil {
            return nil, fmt.Errorf("Unable to parse: %s (%s)", err, msgStr)
        }

        dolog(LevelDebug, "[%s->c] %T %+v", p.remoteLabel, msg, msg)
        return msg, nil

    } else {
        return nil, fmt.Errorf("unimplemented")
    }
}

func (c *protocolAdc) Write(msg msgEncodable) {
    adc,ok := msg.(msgAdcTypeKeyEncodable)
    if !ok {
        panic("command not fit for adc")
    }
    dolog(LevelDebug, "[c->%s] %T %+v", c.remoteLabel, msg, msg)
    c.sendChan <- []byte(adc.AdcTypeEncode(adc.AdcKeyEncode()))
}

type msgAdcTypeDecodable interface {
    AdcTypeDecode(msg string) (int,error)
}

type msgAdcTypeEncodable interface {
    AdcTypeEncode(keyEncoded string) string
}

type msgAdcKeyDecodable interface {
    AdcKeyDecode(args string) error
}

type msgAdcKeyEncodable interface {
    AdcKeyEncode() string
}

type msgAdcTypeKeyDecodable interface {
    msgAdcTypeDecodable
    msgAdcKeyDecodable
}

type msgAdcTypeKeyEncodable interface {
    msgAdcTypeEncodable
    msgAdcKeyEncodable
}

type msgAdcTypeB struct {
    SessionId string
}

func (t *msgAdcTypeB) AdcTypeDecode(msg string) (int,error) {
    matches := reAdcTypeB.FindStringSubmatch(msg)
    if matches == nil {
        return 0, errorArgsFormat
    }
    t.SessionId = matches[1]
    return len(matches[0]), nil
}

func (t *msgAdcTypeB) AdcTypeEncode(keyEncoded string) string {
    return "B" + keyEncoded[:3] + " " + t.SessionId + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeC struct {}

func (t *msgAdcTypeC) AdcTypeDecode(msg string) (int,error) {
    return 0, nil
}

func (t *msgAdcTypeC) AdcTypeEncode(keyEncoded string) string {
    return "C" + keyEncoded[:3] + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeD struct {
    AuthorId string
    TargetId string
}

func (t *msgAdcTypeD) AdcTypeDecode(msg string) (int,error) {
    matches := reAdcTypeD.FindStringSubmatch(msg)
    if matches == nil {
        return 0, errorArgsFormat
    }
    t.AuthorId, t.TargetId = matches[1], matches[2]
    return len(matches[0]), nil
}

func (t *msgAdcTypeD) AdcTypeEncode(keyEncoded string) string {
    return "D" + keyEncoded[:3] + " " + t.AuthorId + " " + t.TargetId + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeE struct {}

type msgAdcTypeF struct {
    SessionId string
    RequiredFeatures map[string]struct{}
    ExcludedFeatures map[string]struct{}
}

func (t *msgAdcTypeF) AdcTypeDecode(msg string) (int,error) {
    matches := reAdcTypeF.FindStringSubmatch(msg)
    if matches == nil {
        return 0, errorArgsFormat
    }
    t.SessionId = matches[1]

    t.RequiredFeatures = make(map[string]struct{})
    t.ExcludedFeatures = make(map[string]struct{})
    features := matches[2]
    for {
        pos := 1
        for pos < len(features) && features[pos] != '+' && features[pos] != '-' {
            pos++
        }
        if features[0] == '+' {
            t.RequiredFeatures[features[1:pos]] = struct{}{}
        } else {
            t.ExcludedFeatures[features[1:pos]] = struct{}{}
        }
        features = features[pos:]
        if len(features) == 0 {
            break
        }
    }
    return len(matches[0]), nil
}

func (t *msgAdcTypeF) AdcTypeEncode(keyEncoded string) string {
    ret := "F" + keyEncoded[:3] + " " + t.SessionId + " "
    for feat,_ := range t.RequiredFeatures {
        ret += "+" + feat
    }
    for feat,_ := range t.ExcludedFeatures {
        ret += "-" + feat
    }
    ret += " " + keyEncoded[3:] + "\n"
    return ret
}

type msgAdcTypeH struct {}

func (t *msgAdcTypeH) AdcTypeEncode(keyEncoded string) string {
    return "H" + keyEncoded[:3] + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeI struct {}

func (t *msgAdcTypeI) AdcTypeDecode(msg string) (int,error) {
    return 0, nil
}

type msgAdcTypeU struct {
    ClientId []byte
}

func (t *msgAdcTypeU) AdcTypeEncode(keyEncoded string) string {
    return "U" + keyEncoded[:3] + " " + dcBase32Encode(t.ClientId) + " " + keyEncoded[3:] + "\n"
}

func (t *msgAdcTypeU) AdcTypeDecode(msg string) (int,error) {
    matches := reAdcTypeU.FindStringSubmatch(msg)
    if matches == nil {
        return 0, errorArgsFormat
    }
    t.ClientId = dcBase32Decode(matches[1])
    return len(matches[0]), nil
}

type msgAdcKeyConnectToMe struct {
    Protocol string
    TcpPort uint
    Token string
}

func (m *msgAdcKeyConnectToMe) AdcKeyEncode() string {
    return "CTM" + " " + m.Protocol + " " + numtoa(m.TcpPort) + " " + m.Token
}

func (m *msgAdcKeyConnectToMe) AdcKeyDecode(args string) error {
    matches := reAdcConnectToMe.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Protocol, m.TcpPort, m.Token = matches[1], atoui(matches[2]), matches[3]
    return nil
}

type msgAdcKeyCommand struct {
    Cmds []string
}

func (m *msgAdcKeyCommand) AdcKeyDecode(args string) error {
    for _,cmd := range strings.Split(args, " ") {
        m.Cmds = append(m.Cmds, adcUnescape(cmd))
    }
    return nil
}

type msgAdcKeyGetPass struct {
    Data []byte
}

func (m *msgAdcKeyGetPass) AdcKeyDecode(args string) error {
    matches := reAdcGetPass.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Data = dcBase32Decode(args)
    return nil
}

type msgAdcKeyInfos struct {
    Fields  map[string]string
}

func (m *msgAdcKeyInfos) AdcKeyDecode(args string) error {
    m.Fields = adcFieldsDecode(args)
    return nil
}

func (m *msgAdcKeyInfos) AdcKeyEncode() string {
    return "INF" + adcFieldsEncode(m.Fields)
}

type msgAdcKeyMessage struct {
    Content string
    Flags string
}

func (m *msgAdcKeyMessage) AdcKeyEncode() string {
    ret := "MSG" + adcEscape(m.Content)
    if m.Flags != "" {
        ret += " " + m.Flags
    }
    return ret
}

func (m *msgAdcKeyMessage) AdcKeyDecode(args string) error {
    argss := strings.Split(args, " ")
    m.Content = adcUnescape(argss[0])
    if len(argss) > 1 {
        m.Flags = argss[1]
    }
    return nil
}

type msgAdcKeyPass struct {
    Data []byte
}

func (m *msgAdcKeyPass) AdcKeyEncode() string {
    return "PAS" + dcBase32Encode(m.Data)
}

type msgAdcKeyQuit struct {
    SessionId   string
    Reason      string
}

func (m *msgAdcKeyQuit) AdcKeyDecode(args string) error {
    matches := reAdcQuit.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.SessionId, m.Reason = matches[1], adcUnescape(matches[3])
    return nil
}

type msgAdcKeyRevConnectToMe struct {
    Protocol string
    Token string
}

func (m *msgAdcKeyRevConnectToMe) AdcKeyEncode() string {
    return "RCM" + " " + m.Protocol + " " + m.Token
}

func (m *msgAdcKeyRevConnectToMe) AdcKeyDecode(args string) error {
    matches := reAdcRevConnectToMe.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Protocol, m.Token = matches[1], matches[2]
    return nil
}

type msgAdcKeySearchRequest struct {
    Fields map[string]string
}

func (m *msgAdcKeySearchRequest) AdcKeyDecode(args string) error {
    m.Fields = adcFieldsDecode(args)
    return nil
}

func (m *msgAdcKeySearchRequest) AdcKeyEncode() string {
    return "SCH" + adcFieldsEncode(m.Fields)
}

type msgAdcKeySearchResult struct {
    Fields map[string]string
}

func (m *msgAdcKeySearchResult) AdcKeyDecode(args string) error {
    m.Fields = adcFieldsDecode(args)
    return nil
}

func (m *msgAdcKeySearchResult) AdcKeyEncode() string {
    return "RES" + adcFieldsEncode(m.Fields)
}

type msgAdcKeySessionId struct {
    Sid string
}

func (m *msgAdcKeySessionId) AdcKeyDecode(args string) error {
    matches := readcSessionId.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Sid = args
    return nil
}

type msgAdcKeyStatus struct {
    Code        uint
    Message     string
}

func (m *msgAdcKeyStatus) AdcKeyDecode(args string) error {
    matches := reAdcStatus.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Code, m.Message = atoui(matches[1]), adcUnescape(matches[2])
    return nil
}

type msgAdcKeySupports struct {
    Features map[string]struct{}
}

func (m *msgAdcKeySupports) AdcKeyDecode(args string) error {
    m.Features = make(map[string]struct{})
    for _,feat := range strings.Split(args, " ") {
        m.Features[feat] = struct{}{}
    }
    if len(m.Features) == 0 {
        return errorArgsFormat
    }
    return nil
}

func (m *msgAdcKeySupports) AdcKeyEncode() string {
    var out []string
    for feat,_ := range m.Features {
        out = append(out, feat)
    }
    return "SUP" + strings.Join(out, " ")
}

type msgAdcBInfos struct {
    msgAdcTypeB
    msgAdcKeyInfos
}

type msgAdcBMessage struct {
    msgAdcTypeB
    msgAdcKeyMessage
}

type msgAdcBSearchRequest struct {
    msgAdcTypeB
    msgAdcKeySearchRequest
}

type msgAdcCInfos struct {
    msgAdcTypeC
    msgAdcKeyInfos
}

type msgAdcCSupports struct {
    msgAdcTypeC
    msgAdcKeySupports
}

type msgAdcCStatus struct {
    msgAdcTypeC
    msgAdcKeyStatus
}

type msgAdcDConnectToMe struct {
    msgAdcTypeD
    msgAdcKeyConnectToMe
}

type msgAdcDMessage struct {
    msgAdcTypeD
    msgAdcKeyMessage
}

type msgAdcDRevConnectToMe struct {
    msgAdcTypeD
    msgAdcKeyRevConnectToMe
}

type msgAdcDSearchResult struct {
    msgAdcTypeD
    msgAdcKeySearchResult
}

type msgAdcFSearchRequest struct {
    msgAdcTypeF
    msgAdcKeySearchRequest
}

type msgAdcHPass struct {
    msgAdcTypeH
    msgAdcKeyPass
}

type msgAdcHSupports struct {
    msgAdcTypeH
    msgAdcKeySupports
}

type msgAdcICommand struct {
    msgAdcTypeI
    msgAdcKeyCommand
}

type msgAdcIGetPass struct {
    msgAdcTypeI
    msgAdcKeyGetPass
}

type msgAdcIInfos struct {
    msgAdcTypeI
    msgAdcKeyInfos
}

type msgAdcIQuit struct {
    msgAdcTypeI
    msgAdcKeyQuit
}

type msgAdcISessionId struct {
    msgAdcTypeI
    msgAdcKeySessionId
}

type msgAdcIStatus struct {
    msgAdcTypeI
    msgAdcKeyStatus
}

type msgAdcISupports struct {
    msgAdcTypeI
    msgAdcKeySupports
}

type msgAdcUSearchResult struct {
    msgAdcTypeU
    msgAdcKeySearchResult
}
