package dctoolkit

import (
    "fmt"
    "net"
)

type listenerUdp struct {
    client      *Client
    state       string
    listener    net.PacketConn
}

func newListenerUdp(client *Client) error {
    listener,err := net.ListenPacket("udp", fmt.Sprintf(":%d", client.conf.UdpPort))
    if err != nil {
        return err
    }

    client.listenerUdp = &listenerUdp{
        client: client,
        state: "running",
        listener: listener,
    }
    return nil
}

func (u *listenerUdp) terminate() {
    switch u.state {
    case "terminated":
        return

    case "running":
        u.listener.Close()

    default:
        panic(fmt.Errorf("Terminate() unsupported in state '%s'", u.state))
    }
    u.state = "terminated"
}

func (u *listenerUdp) do() {
    defer u.client.wg.Done()

    err := func() error {
        var buf [2048]byte
        for {
            n,_,err := u.listener.ReadFrom(buf[:])
            if err != nil {
                u.client.Safe(func() {
                    if u.state == "terminated" {
                        err = errorTerminated
                    }
                })
                return err
            }
            msgStr := string(buf[:n])

            u.client.Safe(func() {
                sr,err := func() (*SearchResult,error) {
                    if u.client.protoIsAdc == true {
                        if msgStr[len(msgStr)-1] != '\n' {
                            return nil, fmt.Errorf("wrong terminator")
                        }
                        msgStr = msgStr[:len(msgStr)-1]

                        if msgStr[:5] != "URES " {
                            return nil, fmt.Errorf("wrong command")
                        }

                        msg := &msgAdcUSearchResult{}
                        n,err := msg.AdcTypeDecode(msgStr[5:])
                        if err != nil {
                            return nil, fmt.Errorf("unable to decode command type")
                        }

                        err = msg.AdcKeyDecode(msgStr[5+n:])
                        if err != nil {
                            return nil, fmt.Errorf("unable to decode command key")
                        }

                        p := u.client.peerByClientId(msg.ClientId)
                        if p == nil {
                            return nil, fmt.Errorf("unknown author")
                        }

                        return adcMsgToSearchResult(true, p, &msg.msgAdcKeySearchResult), nil

                    } else {
                        if msgStr[len(msgStr)-1] != '|' {
                            return nil, fmt.Errorf("wrong terminator")
                        }
                        msgStr = msgStr[:len(msgStr)-1]

                        matches := reNmdcCommand.FindStringSubmatch(msgStr)
                        if matches == nil {
                            return nil, fmt.Errorf("wrong syntax")
                        }

                        // udp is used only for search results
                        if matches[1] != "SR" {
                            return nil, fmt.Errorf("wrong command")
                        }

                        msg := &msgNmdcSearchResult{}
                        err = msg.NmdcDecode(matches[3])
                        if err != nil {
                            return nil, fmt.Errorf("wrong search result")
                        }

                        p := u.client.peerByNick(msg.Nick)
                        if p == nil {
                            return nil, fmt.Errorf("unknown author")
                        }

                        return nmdcMsgToSearchResult(true, p, msg), nil
                    }
                }()
                if err != nil {
                    dolog(LevelDebug, "[udp] unable to parse: %s", err)
                    return
                }

                u.client.handleSearchResult(sr)
            })
        }
    }()

    u.client.Safe(func() {
        switch u.state {
        case "terminated":
        default:
            dolog(LevelInfo, "ERR: %s", err)
        }
    })
}
