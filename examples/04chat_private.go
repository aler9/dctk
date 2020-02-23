// +build ignore

package main

import (
	"fmt"

	dctk "github.com/aler9/dctoolkit"
)

func main() {
	// connect to hub in passive mode
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:    "nmdc://hubip:411",
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
