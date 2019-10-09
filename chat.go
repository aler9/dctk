package dctoolkit

import (
	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"
)

// MessagePublic publishes a message in the hub public chat.
func (c *Client) MessagePublic(content string) {
	if c.protoIsAdc() {
		c.connHub.conn.Write(&adcBMessage{
			&adc.BroadcastPacket{ID: c.adcSessionId},
			&adc.ChatMessage{Text: content},
		})

	} else {
		c.connHub.conn.Write(&nmdc.ChatMessage{c.conf.Nick, content})
	}
}

// MessagePrivate sends a private message to a specific peer connected to the hub.
func (c *Client) MessagePrivate(dest *Peer, content string) {
	if c.protoIsAdc() {
		c.connHub.conn.Write(&adcDMessage{
			&adc.DirectPacket{ID: c.adcSessionId, To: dest.adcSessionId},
			&adc.ChatMessage{Text: content},
		})

	} else {
		c.connHub.conn.Write(&nmdc.PrivateMessage{
			From: c.conf.Nick,
			Name: c.conf.Nick,
			To:   dest.Nick,
			Text: content,
		})
	}
}

func (c *Client) handlePublicMessage(author *Peer, content string) {
	dolog(LevelInfo, "[PUB] <%s> %s", author.Nick, content)
	if c.OnMessagePublic != nil {
		c.OnMessagePublic(author, content)
	}
}

func (c *Client) handlePrivateMessage(author *Peer, content string) {
	dolog(LevelInfo, "[PRIV] <%s> %s", author.Nick, content)
	if c.OnMessagePrivate != nil {
		c.OnMessagePrivate(author, content)
	}
}
