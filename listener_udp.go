package dctoolkit

import (
	"fmt"
	"net"

	"github.com/gswly/go-dc/nmdc"
)

type listenerUdp struct {
	client             *Client
	terminateRequested bool
	listener           net.PacketConn
}

func newListenerUdp(client *Client) error {
	listener, err := net.ListenPacket("udp", fmt.Sprintf(":%d", client.conf.UdpPort))
	if err != nil {
		return err
	}

	client.listenerUdp = &listenerUdp{
		client:   client,
		listener: listener,
	}
	return nil
}

func (u *listenerUdp) close() {
	if u.terminateRequested == true {
		return
	}
	u.terminateRequested = true
	u.listener.Close()
}

func (u *listenerUdp) do() {
	defer u.client.wg.Done()

	var buf [2048]byte
	for {
		n, _, err := u.listener.ReadFrom(buf[:])
		// listener closed
		if err != nil {
			break
		}

		msgStr := string(buf[:n])

		u.client.Safe(func() {
			err := func() error {
				if u.client.protoIsAdc() {
					if msgStr[len(msgStr)-1] != '\n' {
						return fmt.Errorf("wrong terminator")
					}
					msgStr = msgStr[:len(msgStr)-1]

					if msgStr[:5] != "URES " {
						return fmt.Errorf("wrong command")
					}

					msg := &msgAdcUSearchResult{}
					n, err := msg.AdcTypeDecode(msgStr[5:])
					if err != nil {
						return fmt.Errorf("unable to decode command type")
					}

					err = msg.AdcKeyDecode(msgStr[5+n:])
					if err != nil {
						return fmt.Errorf("unable to decode command key")
					}

					p := u.client.peerByClientId(msg.ClientId)
					if p == nil {
						return fmt.Errorf("unknown author")
					}

					u.client.handleAdcSearchResult(true, p, &msg.msgAdcKeySearchResult)
					return nil

				} else {
					if msgStr[len(msgStr)-1] != '|' {
						return fmt.Errorf("wrong terminator")
					}
					msgStr = msgStr[:len(msgStr)-1]

					matches := reNmdcCommand.FindStringSubmatch(msgStr)
					if matches == nil {
						return fmt.Errorf("wrong syntax")
					}

					// udp is used only for search results
					if matches[1] != "SR" {
						return fmt.Errorf("wrong command")
					}

					msg := &nmdc.SR{}
					err = msg.UnmarshalNMDC(nil, []byte(matches[3]))
					if err != nil {
						return fmt.Errorf("wrong search result")
					}

					u.client.handleNmdcSearchResult(true, msg)
					return nil
				}
			}()
			if err != nil {
				dolog(LevelDebug, "[udp] unable to parse: %s", err)
			}
		})
	}
}
