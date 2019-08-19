package dctoolkit

import (
	"bytes"
	"fmt"
	"net"
	"regexp"

	"github.com/gswly/go-dc/nmdc"
)

var reNmdcAddress = regexp.MustCompile("^(" + reStrIp + "):(" + reStrPort + ")$")
var reNmdcCommand = regexp.MustCompile("(?s)^\\$([a-zA-Z0-9:]+)( (.+))?$")
var reNmdcPublicChat = regexp.MustCompile("(?s)^<(" + reStrNick + "|.+?)> (.+)$") // some very bad hubs also use spaces in public message authors

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
				return &nmdcKeepAlive{}, nil
			}

			if matches := reNmdcCommand.FindStringSubmatch(msgStr); matches != nil {
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
	dolog(LevelDebug, "[c->%s] %T %+v", p.remoteLabel, msg, msg)

	if c, ok := msg.(*nmdc.ChatMessage); ok {
		var buf bytes.Buffer
		if err := c.MarshalNMDC(nil, &buf); err != nil {
			panic(err)
		}
		buf.WriteByte('|')
		p.protocolBase.Write(buf.Bytes())
		return
	}

	if _, ok := msg.(*nmdcKeepAlive); ok {
		p.protocolBase.Write([]byte{'|'})
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
	p.protocolBase.Write(buf.Bytes())
}

type nmdcKeepAlive struct{}
