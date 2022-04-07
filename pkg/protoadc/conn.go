package protoadc

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"strings"

	"github.com/aler9/go-dc/adc"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/protocommon"
)

// standard ADC status codes.
const (
	AdcCodeProtocolUnsupported = 41
	AdcCodeFileNotAvailable    = 51
	AdcCodeSlotsFull           = 53
)

// base32 without padding, which can be one or multiple =
func dcBase32Encode(in []byte) string {
	return strings.TrimRight(base32.StdEncoding.EncodeToString(in), "=")
}

// AdcRandomToken returns a random token.
func AdcRandomToken() string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = chars[rand.Intn(len(chars))]
	}
	return string(buf)
}

// AdcCertFingerprint returns the fingerprint of a certificate.
func AdcCertFingerprint(cert *x509.Certificate) string {
	h := sha256.New()
	h.Write(cert.Raw)
	return "SHA256/" + dcBase32Encode(h.Sum(nil))
}

// Conn is an ADC connection.
type Conn struct {
	*protocommon.BaseConn
}

// NewConn allocates a Conn.
func NewConn(logLevel log.Level, remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool,
) *Conn {
	p := &Conn{
		BaseConn: protocommon.NewBaseConn(logLevel, remoteLabel,
			nconn, applyReadTimeout, applyWriteTimeout, '\n'),
	}
	return p
}

// Read reads a message.
func (p *Conn) Read() (protocommon.MsgDecodable, error) {
	if !p.BinaryMode() {
		msgStr, err := p.ReadMessage()
		if err != nil {
			return nil, err
		}

		msg, err := func() (protocommon.MsgDecodable, error) {
			if len(msgStr) == 0 {
				return &AdcKeepAlive{}, nil
			}

			pkt, err := adc.DecodePacket([]byte(msgStr + "\n"))
			if err != nil {
				return nil, err
			}

			msg := func() interface{} {
				switch tpkt := pkt.(type) {
				case *adc.BroadcastPacket:
					switch msg := pkt.Message().(type) {
					case adc.UserInfo:
						return &AdcBInfos{tpkt, &msg}
					case adc.ChatMessage:
						return &AdcBMessage{tpkt, &msg}
					case adc.SearchRequest:
						return &AdcBSearchRequest{tpkt, &msg}
					}

				case *adc.ClientPacket:
					switch msg := pkt.Message().(type) {
					case adc.GetRequest:
						return &AdcCGetFile{tpkt, &msg}
					case adc.UserInfo:
						return &AdcCInfos{tpkt, &msg}
					case adc.GetResponse:
						return &AdcCSendFile{tpkt, &msg}
					case adc.Supported:
						return &AdcCSupports{tpkt, &msg}
					case adc.Status:
						return &AdcCStatus{tpkt, &msg}
					}

				case *adc.DirectPacket:
					switch msg := pkt.Message().(type) {
					case adc.ConnectRequest:
						return &AdcDConnectToMe{tpkt, &msg}
					case adc.ChatMessage:
						return &AdcDMessage{tpkt, &msg}
					case adc.RevConnectRequest:
						return &AdcDRevConnectToMe{tpkt, &msg}
					case adc.SearchResult:
						return &AdcDSearchResult{tpkt, &msg}
					}

				case *adc.FeaturePacket:
					if msg, ok := pkt.Message().(adc.SearchRequest); ok {
						return &AdcFSearchRequest{tpkt, &msg}
					}

				case *adc.HubPacket:
					switch msg := pkt.Message().(type) {
					case adc.Password:
						return &AdcHPass{tpkt, &msg}
					case adc.Supported:
						return &AdcHSupports{tpkt, &msg}
					}

				case *adc.InfoPacket:
					switch msg := pkt.Message().(type) {
					case adc.UserCommand:
						return &AdcICommand{tpkt, &msg}
					case adc.GetPassword:
						return &AdcIGetPass{tpkt, &msg}
					case adc.HubInfo:
						return &AdcIInfos{tpkt, &msg}
					case adc.ChatMessage:
						return &AdcIMsg{tpkt, &msg}
					case adc.Disconnect:
						return &AdcIQuit{tpkt, &msg}
					case adc.SIDAssign:
						return &AdcISessionID{tpkt, &msg}
					case adc.Status:
						return &AdcIStatus{tpkt, &msg}
					case adc.Supported:
						return &AdcISupports{tpkt, &msg}
					case adc.ZOn:
						return &AdcIZon{tpkt, &msg}
					}
				}
				return nil
			}()
			if msg == nil {
				return nil, fmt.Errorf("unsupported message")
			}

			return msg, nil
		}()
		if err != nil {
			return nil, fmt.Errorf("Unable to parse: %s (%s)", err, msgStr)
		}

		log.Log(p.LogLevel(), log.LevelDebug, "[%s->c] %T %+v", p.RemoteLabel(), msg, msg)
		return msg, nil
	}

	buf, err := p.ReadBinary()
	if err != nil {
		return nil, err
	}

	return &protocommon.MsgBinary{buf}, nil //nolint:govet
}

// Write writes a message.
func (p *Conn) Write(pktMsg protocommon.MsgEncodable) {
	log.Log(p.LogLevel(), log.LevelDebug, "[c->%s] %T %+v", p.RemoteLabel(), pktMsg, pktMsg)

	pkt := reflect.ValueOf(pktMsg).Elem().FieldByName("Pkt").Interface().(adc.Packet)
	msg := reflect.ValueOf(pktMsg).Elem().FieldByName("Msg").Interface().(adc.Message)

	pkt.SetMessage(msg)
	var buf bytes.Buffer
	if err := pkt.MarshalPacketADC(&buf); err != nil {
		panic(err)
	}

	p.BaseConn.Write(buf.Bytes())
}

// AdcKeepAlive is an ADC keepalive.
type AdcKeepAlive struct{}

// AdcKeyEncode implements adc.Message.
func (*AdcKeepAlive) AdcKeyEncode() string {
	return ""
}

// AdcTypeEncode implements adc.Message.
func (*AdcKeepAlive) AdcTypeEncode(keyEncoded string) string {
	return ""
}

// AdcBInfos is the BINF message.
type AdcBInfos struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.UserInfo
}

// AdcBMessage is the BMSG message.
type AdcBMessage struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.ChatMessage
}

// AdcBSearchRequest is the BSCH message.
type AdcBSearchRequest struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.SearchRequest
}

// AdcCGetFile is the CGET message.
type AdcCGetFile struct {
	Pkt *adc.ClientPacket
	Msg *adc.GetRequest
}

// AdcCInfos is the CINF message.
type AdcCInfos struct {
	Pkt *adc.ClientPacket
	Msg *adc.UserInfo
}

// AdcCSendFile is the CSNF message.
type AdcCSendFile struct {
	Pkt *adc.ClientPacket
	Msg *adc.GetResponse
}

// AdcCStatus is the CSTA message.
type AdcCStatus struct {
	Pkt *adc.ClientPacket
	Msg *adc.Status
}

// AdcCSupports is the CSUP message.
type AdcCSupports struct {
	Pkt *adc.ClientPacket
	Msg *adc.Supported
}

// AdcDConnectToMe is the DCTM message.
type AdcDConnectToMe struct {
	Pkt *adc.DirectPacket
	Msg *adc.ConnectRequest
}

// AdcDMessage is the DMSG message.
type AdcDMessage struct {
	Pkt *adc.DirectPacket
	Msg *adc.ChatMessage
}

// AdcDRevConnectToMe is the DRCM message.
type AdcDRevConnectToMe struct {
	Pkt *adc.DirectPacket
	Msg *adc.RevConnectRequest
}

// AdcDSearchResult is the DRES message.
type AdcDSearchResult struct {
	Pkt *adc.DirectPacket
	Msg *adc.SearchResult
}

// AdcDStatus is the DSTA message.
type AdcDStatus struct {
	Pkt *adc.DirectPacket
	Msg *adc.Status
}

// AdcFSearchRequest is the FSCH message.
type AdcFSearchRequest struct {
	Pkt *adc.FeaturePacket
	Msg *adc.SearchRequest
}

// AdcHPass is the HPAS message.
type AdcHPass struct {
	Pkt *adc.HubPacket
	Msg *adc.Password
}

// AdcHSupports is the HSUP message.
type AdcHSupports struct {
	Pkt *adc.HubPacket
	Msg *adc.Supported
}

// AdcICommand is the ICMD message.
type AdcICommand struct {
	Pkt *adc.InfoPacket
	Msg *adc.UserCommand
}

// AdcIGetPass is the IGPA message.
type AdcIGetPass struct {
	Pkt *adc.InfoPacket
	Msg *adc.GetPassword
}

// AdcIInfos is the IINF message.
type AdcIInfos struct {
	Pkt *adc.InfoPacket
	Msg *adc.HubInfo
}

// AdcIMsg is the IMSG message.
type AdcIMsg struct {
	*adc.InfoPacket
	Msg *adc.ChatMessage
}

// AdcIQuit is the IQUI message.
type AdcIQuit struct {
	Pkt *adc.InfoPacket
	Msg *adc.Disconnect
}

// AdcISessionID is the ISID message.
type AdcISessionID struct {
	Pkt *adc.InfoPacket
	Msg *adc.SIDAssign
}

// AdcIStatus is the ISTA message.
type AdcIStatus struct {
	Pkt *adc.InfoPacket
	Msg *adc.Status
}

// AdcISupports is the ISUP message.
type AdcISupports struct {
	*adc.InfoPacket
	Msg *adc.Supported
}

// AdcIZon is the IZON message.
type AdcIZon struct {
	Pkt *adc.InfoPacket
	Msg *adc.ZOn
}

// AdcUSearchResult is the URES message.
type AdcUSearchResult struct {
	Pkt *adc.UDPPacket
	Msg *adc.SearchResult
}
