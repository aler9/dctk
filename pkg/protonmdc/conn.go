package protonmdc

import (
	"bytes"
	"fmt"
	"net"
	"regexp"

	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/protocommon"
)

// ReNmdcAddress is the regex to parse a NMDC address.
var ReNmdcAddress = regexp.MustCompile("^(" + protocommon.ReStrIP + "):(" + protocommon.ReStrPort + ")$")

// ReNmdcCommand is the regex to parse a NMDC command
var ReNmdcCommand = regexp.MustCompile(`(?s)^\$([a-zA-Z0-9:]+)( (.+))?$`)

// some very bad hubs also use spaces in public message authors
var reNmdcPublicChat = regexp.MustCompile("(?s)^<(" + protocommon.ReStrNick + "|.+?)> (.+)$")

// Conn is a NMDC connection.
type Conn struct {
	*protocommon.BaseConn
}

// NewConn allocates a Conn.
func NewConn(logLevel log.Level, remoteLabel string, nconn net.Conn,
	applyReadTimeout bool, applyWriteTimeout bool,
) *Conn {
	p := &Conn{
		BaseConn: protocommon.NewBaseConn(logLevel, remoteLabel,
			nconn, applyReadTimeout, applyWriteTimeout, '|'),
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
func (p *Conn) Write(msg protocommon.MsgEncodable) {
	log.Log(p.LogLevel(), log.LevelDebug, "[c->%s] %T %+v", p.RemoteLabel(), msg, msg)

	if c, ok := msg.(*nmdc.ChatMessage); ok {
		var buf bytes.Buffer
		if err := c.MarshalNMDC(nil, &buf); err != nil {
			panic(err)
		}
		buf.WriteByte('|')
		p.BaseConn.Write(buf.Bytes())
		return
	}

	if _, ok := msg.(*NmdcKeepAlive); ok {
		p.BaseConn.Write([]byte{'|'})
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
	p.BaseConn.Write(buf.Bytes())
}

// NmdcKeepAlive is a NMDC keepalive.
type NmdcKeepAlive struct{}
