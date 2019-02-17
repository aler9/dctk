package dctoolkit

import (
)

// Peer represents a client connected to a Hub.
type Peer struct {
    // peer nickname
    Nick            string
    // peer description, if provided
    Description     string
    // peer email, if provided
    Email           string
    // whether peer is a bot
    IsBot           bool
    // whether peer is a operator
    IsOperator      bool
    // client used by peer (in NMDC this could be hidden)
    Client          string
    // version of client (in NMDC this could be hidden)
    Version         string
    // overall size of files shared by peer
    ShareSize       uint64
    // whether peer is in passive mode (in NMDC this could be hidden)
    IsPassive       bool
    // peer ip, if sent by hub
    Ip              string

    adcSessionId    string
    adcClientId     []byte
    adcSupports     map[string]struct{}
    adcUdpPort      uint
    nmdcConnection  string
    nmdcStatusByte  byte
}

func (p *Peer) nmdcSupportTls() bool {
    // we check only for bit 4
    return (p.nmdcStatusByte & (0x01 << 4)) == (0x01 << 4)
}

// Peers returns a map containing all the peers connected to current hub.
func (c *Client) Peers() map[string]*Peer {
    return c.peers
}

func (c *Client) peerByNick(nick string) *Peer {
    if p,ok := c.peers[nick]; ok {
        return p
    }
    return nil
}

func (c *Client) peerBySessionId(sessionId string) *Peer {
    for _,p := range c.peers {
        if p.adcSessionId == sessionId {
            return p
        }
    }
    return nil
}

func (c *Client) peerByClientId(clientId []byte) *Peer {
    for _,p := range c.peers {
        if string(p.adcClientId) == string(clientId) {
            return p
        }
    }
    return nil
}

func (c *Client) peerRequestConnection(peer *Peer) {
    if c.conf.IsPassive == false {
        c.peerConnectToMe(peer)
    } else {
        c.peerRevConnectToMe(peer)
    }
}

func (c *Client) peerConnectToMe(peer *Peer) {
    if c.protoIsAdc == true {
        c.connHub.conn.Write(&msgAdcDConnectToMe{
            msgAdcTypeD{ c.sessionId, peer.adcSessionId },
            msgAdcKeyConnectToMe{ "ADC/1.0", c.conf.TcpPort, "123456789" },
        })

    } else {
        c.connHub.conn.Write(&msgNmdcConnectToMe{
            Target: peer.Nick,
            Ip: c.ip,
            Port: func() uint {
                if c.conf.PeerEncryptionMode != DisableEncryption && peer.nmdcSupportTls() {
                    return c.conf.TcpTlsPort
                }
                return c.conf.TcpPort
            }(),
            Encrypted: (c.conf.PeerEncryptionMode != DisableEncryption && peer.nmdcSupportTls()),
        })
    }
}

func (c *Client) peerRevConnectToMe(peer *Peer) {
    if c.protoIsAdc == true {
        c.connHub.conn.Write(&msgAdcDRevConnectToMe{
            msgAdcTypeD{ c.sessionId, peer.adcSessionId },
            msgAdcKeyRevConnectToMe{ "ADC/1.0", "123456789" },
        })

    } else {
        c.connHub.conn.Write(&msgNmdcRevConnectToMe{
            Author: c.conf.Nick,
            Target: peer.Nick,
        })
    }
}

func (c *Client) handlePeerConnected(peer *Peer) {
    c.peers[peer.Nick] = peer
    dolog(LevelInfo, "[hub] [peer on] %s (%v)", peer.Nick, peer.ShareSize)
    if c.OnPeerConnected != nil {
        c.OnPeerConnected(peer)
    }
}

func (c *Client) handlePeerUpdated(peer *Peer) {
    if c.OnPeerUpdated != nil {
        c.OnPeerUpdated(peer)
    }
}

func (c *Client) handlePeerDisconnected(peer *Peer) {
    delete(c.peers, peer.Nick)
    dolog(LevelInfo, "[hub] [peer off] %s", peer.Nick)
    if c.OnPeerDisconnected != nil {
        c.OnPeerDisconnected(peer)
    }
}

func (c *Client) handlePeerRevConnectToMe(peer *Peer) {
    // we can process RevConnectToMe only in active mode
    if c.conf.IsPassive == false {
        c.peerConnectToMe(peer)
    }
}
