package dctoolkit

import (
	"fmt"

	"github.com/gswly/go-dc/nmdc"
)

// Peer represents a remote client connected to a Hub.
type Peer struct {
	// peer nickname
	Nick string
	// peer description (if provided)
	Description string
	// peer email (if provided)
	Email string
	// whether peer is a bot
	IsBot bool
	// whether peer is a operator
	IsOperator bool
	// client used by peer (in NMDC this could be hidden)
	Client string
	// version of client (in NMDC this could be hidden)
	Version string
	// overall size of files shared by peer
	ShareSize uint64
	// whether peer is in passive mode (in NMDC this could be hidden)
	IsPassive bool
	// peer ip (if provided by both peer and hub)
	Ip string

	adcSessionId   string
	adcClientId    []byte
	adcFingerprint string
	adcSupports    map[string]struct{}
	adcUdpPort     uint
	nmdcConnection string
	nmdcFlag       nmdc.UserFlag
}

// Peers returns a map containing all the peers connected to current hub.
func (c *Client) Peers() map[string]*Peer {
	return c.peers
}

func (c *Client) peerByNick(nick string) *Peer {
	if p, ok := c.peers[nick]; ok {
		return p
	}
	return nil
}

func (c *Client) peerBySessionId(sessionId string) *Peer {
	for _, p := range c.peers {
		if p.adcSessionId == sessionId {
			return p
		}
	}
	return nil
}

func (c *Client) peerByClientId(clientId []byte) *Peer {
	for _, p := range c.peers {
		if string(p.adcClientId) == string(clientId) {
			return p
		}
	}
	return nil
}

func (c *Client) peerSupportsEncryption(p *Peer) bool {
	if c.protoIsAdc() {
		if p.adcFingerprint != "" {
			return true
		}
		if _, ok := p.adcSupports[adcSupportTls]; ok {
			return true
		}
		return false

	} else {
		// we check only for bit 4
		return (p.nmdcFlag & nmdc.FlagTLSDownload) != 0
	}
}

func (c *Client) peerRequestConnection(peer *Peer, adcToken string) {
	if c.conf.IsPassive == false {
		c.peerConnectToMe(peer, adcToken)
	} else {
		c.peerRevConnectToMe(peer, adcToken)
	}
}

func (c *Client) peerConnectToMe(peer *Peer, adcToken string) {
	if c.protoIsAdc() {
		c.connHub.conn.Write(&msgAdcDConnectToMe{
			msgAdcTypeD{c.sessionId, peer.adcSessionId},
			msgAdcKeyConnectToMe{
				func() string {
					if c.conf.PeerEncryptionMode != DisableEncryption && c.peerSupportsEncryption(peer) {
						return adcProtocolEncrypted
					}
					return adcProtocolPlain
				}(),
				func() uint {
					if c.conf.PeerEncryptionMode != DisableEncryption && c.peerSupportsEncryption(peer) {
						return c.conf.TcpTlsPort
					}
					return c.conf.TcpPort
				}(),
				adcToken,
			},
		})

	} else {
		c.connHub.conn.Write(&nmdc.ConnectToMe{
			Targ: peer.Nick,
			Address: fmt.Sprintf("%s:%d", c.ip, func() uint {
				if c.conf.PeerEncryptionMode != DisableEncryption && c.peerSupportsEncryption(peer) {
					return c.conf.TcpTlsPort
				}
				return c.conf.TcpPort
			}()),
			Secure: (c.conf.PeerEncryptionMode != DisableEncryption && c.peerSupportsEncryption(peer)),
		})
	}
}

func (c *Client) peerRevConnectToMe(peer *Peer, adcToken string) {
	if c.protoIsAdc() {
		c.connHub.conn.Write(&msgAdcDRevConnectToMe{
			msgAdcTypeD{c.sessionId, peer.adcSessionId},
			msgAdcKeyRevConnectToMe{
				func() string {
					if c.conf.PeerEncryptionMode != DisableEncryption && c.peerSupportsEncryption(peer) {
						return adcProtocolEncrypted
					}
					return adcProtocolPlain
				}(),
				adcToken,
			},
		})

	} else {
		c.connHub.conn.Write(&nmdc.RevConnectToMe{
			From: c.conf.Nick,
			To:   peer.Nick,
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

func (c *Client) handlePeerRevConnectToMe(peer *Peer, adcToken string) {
	// we can process RevConnectToMe only in active mode
	if c.conf.IsPassive == false {
		c.peerConnectToMe(peer, adcToken)
	}
}
