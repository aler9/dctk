package dctoolkit

import (
	"fmt"
	"net"

	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctoolkit/log"
	"github.com/aler9/dctoolkit/proto"
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

					pkt, err := adc.DecodePacket([]byte(msgStr + "\n"))
					if err != nil {
						return err
					}

					msge := pkt.Message().(adc.SearchResult)
					msg := &msge

					pktMsg := &proto.AdcUSearchResult{
						pkt.(*adc.UDPPacket),
						msg,
					}

					p := u.client.peerByClientId(pktMsg.Pkt.ID)
					if p == nil {
						return fmt.Errorf("unknown author")
					}

					u.client.handleAdcSearchResult(true, p, pktMsg.Msg)
					return nil

				} else {
					if msgStr[len(msgStr)-1] != '|' {
						return fmt.Errorf("wrong terminator")
					}
					msgStr = msgStr[:len(msgStr)-1]

					matches := proto.ReNmdcCommand.FindStringSubmatch(msgStr)
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
				log.Log(u.client.conf.LogLevel, LogLevelDebug, "[udp] unable to parse: %s", err)
			}
		})
	}
}
