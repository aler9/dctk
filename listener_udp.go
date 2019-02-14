package dctoolkit

import (
    "fmt"
    "net"
)

type udpListener struct {
    client      *Client
    state       string
    listener    net.PacketConn
}

func newUdpListener(client *Client) error {
    listener,err := net.ListenPacket("udp", fmt.Sprintf(":%d", client.conf.UdpPort))
    if err != nil {
        return err
    }

    client.udpListener = &udpListener{
        client: client,
        state: "running",
        listener: listener,
    }

    return nil
}

func (u *udpListener) terminate() {
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

func (u *udpListener) do() {
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

            if msgStr[len(msgStr)-1] != '|' {
                dolog(LevelDebug, "unable to parse incoming UDP (1): %s", msgStr)
                continue
            }

            matches := reNmdcCommand.FindStringSubmatch(msgStr[:len(msgStr)-1])
            if matches == nil {
                dolog(LevelDebug, "unable to parse incoming UDP (2): %s", msgStr)
                continue
            }

            // udp is used only for search results
            if matches[1] != "SR" {
                dolog(LevelDebug, "unable to parse incoming UDP (3): %s", msgStr)
                continue
            }

            msg := &msgNmdcSearchResult{}
            err = msg.NmdcDecode(matches[3])
            if err != nil {
                dolog(LevelInfo, "unable to parse incoming search result: %s", msgStr)
            }

            sr := &SearchResult{
                IsActive: true,
                Nick: msg.Nick,
                Path: msg.Path,
                SlotAvail: msg.SlotAvail,
                SlotCount: msg.SlotCount,
                TTH: msg.TTH,
                IsDir: msg.IsDir,
                HubIp: msg.HubIp,
                HubPort: msg.HubPort,
            }
            dolog(LevelInfo, "[search res] %+v", sr)

            u.client.Safe(func() {
                if u.client.OnSearchResult != nil {
                    u.client.OnSearchResult(sr)
                }
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
