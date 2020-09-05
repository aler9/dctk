package proto

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

	"github.com/aler9/dctk/log"
)

const (
	AdcCodeProtocolUnsupported = 41
	AdcCodeFileNotAvailable    = 51
	AdcCodeSlotsFull           = 53
)

// base32 without padding, which can be one or multiple =
func dcBase32Encode(in []byte) string {
	return strings.TrimRight(base32.StdEncoding.EncodeToString(in), "=")
}

func AdcRandomToken() string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = chars[rand.Intn(len(chars))]
	}
	return string(buf)
}

func AdcCertFingerprint(cert *x509.Certificate) string {
	h := sha256.New()
	h.Write(cert.Raw)
	return "SHA256/" + dcBase32Encode(h.Sum(nil))
}

type AdcConn struct {
	*BaseConn
}

func NewAdcConn(logLevel log.Level, remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool) Conn {
	p := &AdcConn{
		BaseConn: newBaseConn(logLevel, remoteLabel,
			nconn, applyReadTimeout, applyWriteTimeout, '\n'),
	}
	return p
}

func (p *AdcConn) Read() (MsgDecodable, error) {
	if p.readBinary == false {
		msgStr, err := p.ReadMessage()
		if err != nil {
			return nil, err
		}

		msg, err := func() (MsgDecodable, error) {
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
					switch msg := pkt.Message().(type) {
					case adc.SearchRequest:
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
						return &AdcISessionId{tpkt, &msg}
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

		log.Log(p.logLevel, log.LevelDebug, "[%s->c] %T %+v", p.remoteLabel, msg, msg)
		return msg, nil

	} else {
		buf, err := p.ReadBinary()
		if err != nil {
			return nil, err
		}
		return &MsgBinary{buf}, nil
	}
}

func (p *AdcConn) Write(pktMsg MsgEncodable) {
	log.Log(p.logLevel, log.LevelDebug, "[c->%s] %T %+v", p.remoteLabel, pktMsg, pktMsg)

	pkt := reflect.ValueOf(pktMsg).Elem().FieldByName("Pkt").Interface().(adc.Packet)
	msg := reflect.ValueOf(pktMsg).Elem().FieldByName("Msg").Interface().(adc.Message)

	pkt.SetMessage(msg)
	var buf bytes.Buffer
	if err := pkt.MarshalPacketADC(&buf); err != nil {
		panic(err)
	}

	p.BaseConn.Write(buf.Bytes())
}

type AdcKeepAlive struct{}

func (*AdcKeepAlive) AdcKeyEncode() string {
	return ""
}

func (*AdcKeepAlive) AdcTypeEncode(keyEncoded string) string {
	return ""
}

type AdcBInfos struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.UserInfo
}

type AdcBMessage struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.ChatMessage
}

type AdcBSearchRequest struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.SearchRequest
}

type AdcCGetFile struct {
	Pkt *adc.ClientPacket
	Msg *adc.GetRequest
}

type AdcCInfos struct {
	Pkt *adc.ClientPacket
	Msg *adc.UserInfo
}

type AdcCSendFile struct {
	Pkt *adc.ClientPacket
	Msg *adc.GetResponse
}

type AdcCStatus struct {
	Pkt *adc.ClientPacket
	Msg *adc.Status
}

type AdcCSupports struct {
	Pkt *adc.ClientPacket
	Msg *adc.Supported
}

type AdcDConnectToMe struct {
	Pkt *adc.DirectPacket
	Msg *adc.ConnectRequest
}

type AdcDMessage struct {
	Pkt *adc.DirectPacket
	Msg *adc.ChatMessage
}

type AdcDRevConnectToMe struct {
	Pkt *adc.DirectPacket
	Msg *adc.RevConnectRequest
}

type AdcDSearchResult struct {
	Pkt *adc.DirectPacket
	Msg *adc.SearchResult
}

type AdcDStatus struct {
	Pkt *adc.DirectPacket
	Msg *adc.Status
}

type AdcFSearchRequest struct {
	Pkt *adc.FeaturePacket
	Msg *adc.SearchRequest
}

type AdcHPass struct {
	Pkt *adc.HubPacket
	Msg *adc.Password
}

type AdcHSupports struct {
	Pkt *adc.HubPacket
	Msg *adc.Supported
}

type AdcICommand struct {
	Pkt *adc.InfoPacket
	Msg *adc.UserCommand
}

type AdcIGetPass struct {
	Pkt *adc.InfoPacket
	Msg *adc.GetPassword
}

type AdcIInfos struct {
	Pkt *adc.InfoPacket
	Msg *adc.HubInfo
}

type AdcIMsg struct {
	*adc.InfoPacket
	Msg *adc.ChatMessage
}

type AdcIQuit struct {
	Pkt *adc.InfoPacket
	Msg *adc.Disconnect
}

type AdcISessionId struct {
	Pkt *adc.InfoPacket
	Msg *adc.SIDAssign
}

type AdcIStatus struct {
	Pkt *adc.InfoPacket
	Msg *adc.Status
}

type AdcISupports struct {
	*adc.InfoPacket
	Msg *adc.Supported
}

type AdcIZon struct {
	Pkt *adc.InfoPacket
	Msg *adc.ZOn
}

type AdcUSearchResult struct {
	Pkt *adc.UDPPacket
	Msg *adc.SearchResult
}
