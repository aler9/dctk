package dctk

import (
	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/proto"
)

// MessagePublic publishes a message in the hub public chat.
func (c *Client) MessagePublic(content string) {
	if c.protoIsAdc() {
		c.hubConn.conn.Write(&proto.AdcBMessage{ //nolint:govet
			&adc.BroadcastPacket{ID: c.adcSessionID},
			&adc.ChatMessage{Text: content},
		})

	} else {
		c.hubConn.conn.Write(&nmdc.ChatMessage{c.conf.Nick, content}) //nolint:govet
	}
}

// MessagePrivate sends a private message to a specific peer connected to the hub.
func (c *Client) MessagePrivate(dest *Peer, content string) {
	if c.protoIsAdc() {
		c.hubConn.conn.Write(&proto.AdcDMessage{ //nolint:govet
			&adc.DirectPacket{ID: c.adcSessionID, To: dest.adcSessionID},
			&adc.ChatMessage{Text: content},
		})

	} else {
		c.hubConn.conn.Write(&nmdc.PrivateMessage{
			From: c.conf.Nick,
			Name: c.conf.Nick,
			To:   dest.Nick,
			Text: content,
		})
	}
}

func (c *Client) handlePublicMessage(author *Peer, content string) {
	log.Log(c.conf.LogLevel, log.LevelInfo, "[PUB] <%s> %s", author.Nick, content)
	if c.OnMessagePublic != nil {
		c.OnMessagePublic(author, content)
	}
}

func (c *Client) handlePrivateMessage(author *Peer, content string) {
	log.Log(c.conf.LogLevel, log.LevelInfo, "[PRIV] <%s> %s", author.Nick, content)
	if c.OnMessagePrivate != nil {
		c.OnMessagePrivate(author, content)
	}
}
