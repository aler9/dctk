package proto

import (
	"bytes"
	"fmt"
	"net"
	"regexp"

	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctoolkit/log"
)

var ReNmdcAddress = regexp.MustCompile("^(" + ReStrIp + "):(" + reStrPort + ")$")
var ReNmdcCommand = regexp.MustCompile("(?s)^\\$([a-zA-Z0-9:]+)( (.+))?$")
var reNmdcPublicChat = regexp.MustCompile("(?s)^<(" + reStrNick + "|.+?)> (.+)$") // some very bad hubs also use spaces in public message authors

type ProtocolNmdc struct {
	*ProtocolBase
}

func NewProtocolNmdc(logLevel log.Level, remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool) Protocol {
	p := &ProtocolNmdc{
		ProtocolBase: newProtocolBase(logLevel, remoteLabel,
			nconn, applyReadTimeout, applyWriteTimeout, '|'),
	}
	return p
}

func (p *ProtocolNmdc) Read() (MsgDecodable, error) {
	if p.readBinary == false {
		msgStr, err := p.ReadMessage()
		if err != nil {
			return nil, err
		}

		msg, err := func() (MsgDecodable, error) {
			if len(msgStr) == 0 {
				return &NmdcKeepAlive{}, nil
			}

			if matches := ReNmdcCommand.FindStringSubmatch(msgStr); matches != nil {
				key, args := matches[1], matches[3]

				cmd := func() nmdc.Message {
					switch key {
					case "ADCGET":
						return &nmdc.ADCGet{}
					case "ADCSND":
						return &nmdc.ADCSnd{}
					case "BadPass":
						return &nmdc.BadPass{}
					case "BotList":
						return &nmdc.BotList{}
					case "ConnectToMe":
						return &nmdc.ConnectToMe{}
					case "Direction":
						return &nmdc.Direction{}
					case "Error":
						return &nmdc.Error{}
					case "ForceMove":
						return &nmdc.ForceMove{}
					case "GetPass":
						return &nmdc.GetPass{}
					case "Hello":
						return &nmdc.Hello{}
					case "HubName":
						return &nmdc.HubName{}
					case "HubIsFull":
						return &nmdc.HubIsFull{}
					case "HubTopic":
						return &nmdc.HubTopic{}
					case "Key":
						return &nmdc.Key{}
					case "Lock":
						return &nmdc.Lock{}
					case "LogedIn":
						return &nmdc.LogedIn{}
					case "MaxedOut":
						return &nmdc.MaxedOut{}
					case "MyINFO":
						return &nmdc.MyINFO{}
					case "MyNick":
						return &nmdc.MyNick{}
					case "OpList":
						return &nmdc.OpList{}
					case "Quit":
						return &nmdc.Quit{}
					case "RevConnectToMe":
						return &nmdc.RevConnectToMe{}
					case "Search":
						return &nmdc.Search{}
					case "SR":
						return &nmdc.SR{}
					case "Supports":
						return &nmdc.Supports{}
					case "To:":
						return &nmdc.PrivateMessage{}
					case "UserCommand":
						return &nmdc.UserCommand{}
					case "UserIP":
						return &nmdc.UserIP{}
					case "ValidateDenide":
						return &nmdc.ValidateDenide{}
					case "ZOn":
						return &nmdc.ZOn{}
					}
					return nil
				}()
				if cmd == nil {
					return nil, fmt.Errorf("unrecognized command")
				}

				err := cmd.UnmarshalNMDC(nil, []byte(args))
				if err != nil {
					return nil, fmt.Errorf("unable to decode arguments: %s", err)
				}
				return cmd, nil
			}

			if matches := reNmdcPublicChat.FindStringSubmatch(msgStr); matches != nil {
				return &nmdc.ChatMessage{Name: matches[1], Text: matches[2]}, nil
			}

			return nil, fmt.Errorf("unknown sequence")
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

func (p *ProtocolNmdc) Write(msg MsgEncodable) {
	log.Log(p.logLevel, log.LevelDebug, "[c->%s] %T %+v", p.remoteLabel, msg, msg)

	if c, ok := msg.(*nmdc.ChatMessage); ok {
		var buf bytes.Buffer
		if err := c.MarshalNMDC(nil, &buf); err != nil {
			panic(err)
		}
		buf.WriteByte('|')
		p.ProtocolBase.Write(buf.Bytes())
		return
	}

	if _, ok := msg.(*NmdcKeepAlive); ok {
		p.ProtocolBase.Write([]byte{'|'})
		return
	}

	msgn, ok := msg.(nmdc.Message)
	if !ok {
		panic(fmt.Errorf("command not fit for nmdc (%T)", msg))
	}

	var buf bytes.Buffer
	buf.WriteByte('$')
	buf.WriteString(msgn.Type())

	var buf2 bytes.Buffer
	err := msgn.MarshalNMDC(nil, &buf2)
	if err != nil {
		panic(err)
	}
	if len(buf2.Bytes()) > 0 {
		buf.WriteByte(' ')
		buf.Write(buf2.Bytes())
	}

	buf.WriteByte('|')
	p.ProtocolBase.Write(buf.Bytes())
}

type NmdcKeepAlive struct{}
