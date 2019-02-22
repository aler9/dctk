package dctoolkit

// MessagePublic publishes a message in the hub public chat.
func (c *Client) MessagePublic(content string) {
	if c.protoIsAdc == true {
		c.connHub.conn.Write(&msgAdcBMessage{
			msgAdcTypeB{c.sessionId},
			msgAdcKeyMessage{Content: content},
		})

	} else {
		c.connHub.conn.Write(&msgNmdcPublicChat{c.conf.Nick, content})
	}
}

// MessagePrivate sends a private message to a specific peer connected to the hub.
func (c *Client) MessagePrivate(dest *Peer, content string) {
	if c.protoIsAdc == true {
		c.connHub.conn.Write(&msgAdcDMessage{
			msgAdcTypeD{c.sessionId, dest.adcSessionId},
			msgAdcKeyMessage{Content: content},
		})

	} else {
		c.connHub.conn.Write(&msgNmdcPrivateChat{c.conf.Nick, dest.Nick, content})
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
