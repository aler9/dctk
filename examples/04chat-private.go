// +build ignore

package main

import (
	"fmt"

	"github.com/aler9/dctk"
)

func main() {
	// connect to hub in passive mode
	client, err := dctk.NewClient(dctk.ClientConf{
		HubURL:    "nmdc://hubip:411",
		Nick:      "mynick",
		IsPassive: true,
	})
	if err != nil {
		panic(err)
	}

	// a private message has been received: reply to sender
	client.OnMessagePrivate = func(p *dctk.Peer, content string) {
		client.MessagePrivate(p, fmt.Sprintf("message received! (%s)", content))
	}

	client.Run()
}
