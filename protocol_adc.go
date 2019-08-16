package dctoolkit

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"math/rand"
	"net"
	"reflect"

	"github.com/gswly/go-dc/adc"
)

const (
	adcCodeProtocolUnsupported = 41
	adcCodeFileNotAvailable    = 51
	adcCodeSlotsFull           = 53
)

func adcRandomToken() string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = chars[rand.Intn(len(chars))]
	}
	return string(buf)
}

func adcCertFingerprint(cert *x509.Certificate) string {
	h := sha256.New()
	h.Write(cert.Raw)
	return "SHA256/" + dcBase32Encode(h.Sum(nil))
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

func (p *protocolAdc) Read() (msgDecodable, error) {
	if p.readBinary == false {
		msgStr, err := p.ReadMessage()
		if err != nil {
			return nil, err
		}

		msg, err := func() (msgDecodable, error) {
			if len(msgStr) == 0 {
				return &adcKeepAlive{}, nil
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
						return &adcBInfos{tpkt, &msg}
					case adc.ChatMessage:
						return &adcBMessage{tpkt, &msg}
					case adc.SearchRequest:
						return &adcBSearchRequest{tpkt, &msg}
					}

				case *adc.ClientPacket:
					switch msg := pkt.Message().(type) {
					case adc.GetRequest:
						return &adcCGetFile{tpkt, &msg}
					case adc.UserInfo:
						return &adcCInfos{tpkt, &msg}
					case adc.GetResponse:
						return &adcCSendFile{tpkt, &msg}
					case adc.Supported:
						return &adcCSupports{tpkt, &msg}
					case adc.Status:
						return &adcCStatus{tpkt, &msg}
					}

				case *adc.DirectPacket:
					switch msg := pkt.Message().(type) {
					case adc.ConnectRequest:
						return &adcDConnectToMe{tpkt, &msg}
					case adc.ChatMessage:
						return &adcDMessage{tpkt, &msg}
					case adc.RevConnectRequest:
						return &adcDRevConnectToMe{tpkt, &msg}
					case adc.SearchResult:
						return &adcDSearchResult{tpkt, &msg}
					}

				case *adc.FeaturePacket:
					switch msg := pkt.Message().(type) {
					case adc.SearchRequest:
						return &adcFSearchRequest{tpkt, &msg}
					}

				case *adc.HubPacket:
					switch msg := pkt.Message().(type) {
					case adc.Password:
						return &adcHPass{tpkt, &msg}
					case adc.Supported:
						return &adcHSupports{tpkt, &msg}
					}

				case *adc.InfoPacket:
					switch msg := pkt.Message().(type) {
					case adc.UserCommand:
						return &adcICommand{tpkt, &msg}
					case adc.GetPassword:
						return &adcIGetPass{tpkt, &msg}
					case adc.HubInfo:
						return &adcIInfos{tpkt, &msg}
					case adc.ChatMessage:
						return &adcIMsg{tpkt, &msg}
					case adc.Disconnect:
						return &adcIQuit{tpkt, &msg}
					case adc.SIDAssign:
						return &adcISessionId{tpkt, &msg}
					case adc.Status:
						return &adcIStatus{tpkt, &msg}
					case adc.Supported:
						return &adcISupports{tpkt, &msg}
					case adc.ZOn:
						return &adcIZon{tpkt, &msg}
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

func (p *protocolAdc) Write(pktMsg msgEncodable) {
	dolog(LevelDebug, "[c->%s] %T %+v", p.remoteLabel, pktMsg, pktMsg)

	pkt := reflect.ValueOf(pktMsg).Elem().FieldByName("Pkt").Interface().(adc.Packet)
	msg := reflect.ValueOf(pktMsg).Elem().FieldByName("Msg").Interface().(adc.Message)

	pkt.SetMessage(msg)
	var buf bytes.Buffer
	if err := pkt.MarshalPacketADC(&buf); err != nil {
		panic(err)
	}

	p.protocolBase.Write(buf.Bytes())
}

type adcKeepAlive struct{}

func (*adcKeepAlive) AdcKeyEncode() string {
	return ""
}

func (*adcKeepAlive) AdcTypeEncode(keyEncoded string) string {
	return ""
}

type adcBInfos struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.UserInfo
}

type adcBMessage struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.ChatMessage
}

type adcBSearchRequest struct {
	Pkt *adc.BroadcastPacket
	Msg *adc.SearchRequest
}

type adcCGetFile struct {
	Pkt *adc.ClientPacket
	Msg *adc.GetRequest
}

type adcCInfos struct {
	Pkt *adc.ClientPacket
	Msg *adc.UserInfo
}

type adcCSendFile struct {
	Pkt *adc.ClientPacket
	Msg *adc.GetResponse
}

type adcCStatus struct {
	Pkt *adc.ClientPacket
	Msg *adc.Status
}

type adcCSupports struct {
	Pkt *adc.ClientPacket
	Msg *adc.Supported
}

type adcDConnectToMe struct {
	Pkt *adc.DirectPacket
	Msg *adc.ConnectRequest
}

type adcDMessage struct {
	Pkt *adc.DirectPacket
	Msg *adc.ChatMessage
}

type adcDRevConnectToMe struct {
	Pkt *adc.DirectPacket
	Msg *adc.RevConnectRequest
}

type adcDSearchResult struct {
	Pkt *adc.DirectPacket
	Msg *adc.SearchResult
}

type adcDStatus struct {
	Pkt *adc.DirectPacket
	Msg *adc.Status
}

type adcFSearchRequest struct {
	Pkt *adc.FeaturePacket
	Msg *adc.SearchRequest
}

type adcHPass struct {
	Pkt *adc.HubPacket
	Msg *adc.Password
}

type adcHSupports struct {
	Pkt *adc.HubPacket
	Msg *adc.Supported
}

type adcICommand struct {
	Pkt *adc.InfoPacket
	Msg *adc.UserCommand
}

type adcIGetPass struct {
	Pkt *adc.InfoPacket
	Msg *adc.GetPassword
}

type adcIInfos struct {
	Pkt *adc.InfoPacket
	Msg *adc.HubInfo
}

type adcIMsg struct {
	*adc.InfoPacket
	Msg *adc.ChatMessage
}

type adcIQuit struct {
	Pkt *adc.InfoPacket
	Msg *adc.Disconnect
}

type adcISessionId struct {
	Pkt *adc.InfoPacket
	Msg *adc.SIDAssign
}

type adcIStatus struct {
	Pkt *adc.InfoPacket
	Msg *adc.Status
}

type adcISupports struct {
	*adc.InfoPacket
	Msg *adc.Supported
}

type adcIZon struct {
	Pkt *adc.InfoPacket
	Msg *adc.ZOn
}

type adcUSearchResult struct {
	Pkt *adc.UDPPacket
	Msg *adc.SearchResult
}
