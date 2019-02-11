package dctoolkit

import (
    "fmt"
    "strings"
    "regexp"
)

const (
    adcInfoSoftware             = "AP"
    adcInfoCategory             = "CT"
    adcInfoDescription          = "DE"
    adcInfoEmail                = "EM"
    adcInfoClientId             = "ID"
    adcInfoHubUnregisteredCount = "HN"
    adcInfoHubRegisteredCount   = "HR"
    adcInfoHubOperatorCount     = "HO"
    adcInfoIp                   = "I4"
    adcInfoName                 = "NI"
    adcInfoPrivateId            = "PD"
    adcInfoShareSize            = "SS"
    adcInfoShareCount           = "SF"
    adcInfoSupports             = "SU"
    adcInfoUdpPort              = "U4"
    adcInfoVersion              = "VE"
)

var reAdcTypeB = regexp.MustCompile("^(.){4} ([A-Z0-9]{4}) ")
var reAdcTypeD = regexp.MustCompile("^(.){4} ([A-Z0-9]{4}) ([A-Z0-9]{4}) ")

var reAdcGetPass = regexp.MustCompile("^[A-Z0-9]{3,}$")
var reAdcQuit = regexp.MustCompile("^([A-Z0-9]+) (.+)$")
var reAdcSessionId = regexp.MustCompile("^[A-Z0-9]{4}$")
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
    t.SessionId = matches[2]
    return len(matches[0]), nil
}

func (t *msgAdcTypeB) AdcTypeEncode(keyEncoded string) string {
    return "B" + keyEncoded[:3] + " " + t.SessionId + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeC struct {}

type msgAdcTypeD struct {
    AuthorId string
    TargetId string
}

func (t *msgAdcTypeD) AdcTypeDecode(msg string) (int,error) {
    matches := reAdcTypeD.FindStringSubmatch(msg)
    if matches == nil {
        return 0, errorArgsFormat
    }
    t.AuthorId, t.TargetId = matches[2], matches[3]
    return len(matches[0]), nil
}

func (t *msgAdcTypeD) AdcTypeEncode(keyEncoded string) string {
    return "D" + keyEncoded[:3] + " " + t.AuthorId + " " + t.TargetId + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeE struct {}

type msgAdcTypeF struct {}

type msgAdcTypeH struct {}

func (t *msgAdcTypeH) AdcTypeEncode(keyEncoded string) string {
    return "H" + keyEncoded[:3] + " " + keyEncoded[3:] + "\n"
}

type msgAdcTypeI struct {}

func (t *msgAdcTypeI) AdcTypeDecode(msg string) (int,error) {
    return 5, nil
}

type msgAdcTypeU struct {}

type msgAdcKeyGetPass struct {
    Data []byte
}

func (m *msgAdcKeyGetPass) AdcKeyDecode(args string) error {
    matches := reAdcGetPass.FindStringSubmatch(args)
    if matches == nil {
        return errorArgsFormat
    }
    m.Data = adcBase32Decode(args)
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

type msgAdcKeyInfos struct {
    Fields  map[string]string
}

func (m *msgAdcKeyInfos) AdcKeyDecode(args string) error {
    m.Fields = make(map[string]string)
    for _,arg := range strings.Split(args, " ") {
        if len(arg) < 2 {
            return errorArgsFormat
        }
        m.Fields[arg[:2]] = adcUnescape(arg[2:])
    }
    if _,ok := m.Fields["NI"]; !ok {
        return fmt.Errorf("NI not sent")
    }
    return nil
}

func (m *msgAdcKeyInfos) AdcKeyEncode() string {
    var fields []string
    for key,val := range m.Fields {
        fields = append(fields, key + adcEscape(val))
    }
    return "INF" + strings.Join(fields, " ")
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
    return "PAS" + adcBase32Encode(m.Data)
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
    m.SessionId, m.Reason = matches[1], adcUnescape(matches[2])
    return nil
}

type msgAdcKeySearchRequest struct {
    Fields map[string]string
}

func (m *msgAdcKeySearchRequest) AdcKeyDecode(args string) error {
    m.Fields = make(map[string]string)
    for _,arg := range strings.Split(args, " ") {
        if len(arg) < 2 {
            return errorArgsFormat
        }
        m.Fields[arg[:2]] = adcUnescape(arg[2:])
    }
    return nil
}

func (m *msgAdcKeySearchRequest) AdcKeyEncode() string {
    var fields []string
    for key,val := range m.Fields {
        fields = append(fields, key + adcEscape(val))
    }
    return "SCH" + strings.Join(fields, " ")
}

type msgAdcKeySessionId struct {
    Sid string
}

func (m *msgAdcKeySessionId) AdcKeyDecode(args string) error {
    matches := reAdcSessionId.FindStringSubmatch(args)
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
    Features []string
}

func (m *msgAdcKeySupports) AdcKeyDecode(args string) error {
    m.Features = strings.Split(args, " ")
    if len(m.Features) == 0 {
        return errorArgsFormat
    }
    return nil
}

func (m *msgAdcKeySupports) AdcKeyEncode() string {
    return "SUP" + strings.Join(m.Features, " ")
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

type msgAdcHPass struct {
    msgAdcTypeH
    msgAdcKeyPass
}

type msgAdcDMessage struct {
    msgAdcTypeD
    msgAdcKeyMessage
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
