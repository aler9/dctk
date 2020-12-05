package dctk

import (
	"fmt"
	"net"

	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/protoadc"
	"github.com/aler9/dctk/pkg/protonmdc"
)

type listenerUDP struct {
	client             *Client
	terminateRequested bool
	listener           net.PacketConn
}

func newListenerUDP(client *Client) error {
	listener, err := net.ListenPacket("udp", fmt.Sprintf(":%d", client.conf.UDPPort))
	if err != nil {
		return err
	}

	client.listenerUDP = &listenerUDP{
		client:   client,
		listener: listener,
	}
	return nil
}

func (u *listenerUDP) close() {
	if u.terminateRequested {
		return
	}
	u.terminateRequested = true
	u.listener.Close()
}

func (u *listenerUDP) do() {
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

					pkt, err := adc.DecodePacket([]byte(msgStr + "\n"))
					if err != nil {
						return err
					}

					msge := pkt.Message().(adc.SearchResult)
					msg := &msge

					pktMsg := &protoadc.AdcUSearchResult{ //nolint:govet
						pkt.(*adc.UDPPacket),
						msg,
					}

					p := u.client.peerByClientID(pktMsg.Pkt.ID)
					if p == nil {
						return fmt.Errorf("unknown author")
					}

					u.client.handleAdcSearchResult(true, p, pktMsg.Msg)
					return nil

				}

				if msgStr[len(msgStr)-1] != '|' {
					return fmt.Errorf("wrong terminator")
				}
				msgStr = msgStr[:len(msgStr)-1]

				matches := protonmdc.ReNmdcCommand.FindStringSubmatch(msgStr)
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
			}()
			if err != nil {
				log.Log(u.client.conf.LogLevel, log.LevelDebug, "[udp] unable to parse: %s", err)
			}
		})
	}
}
