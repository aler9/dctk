package main

import (
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

	// a public message has been received: reply
	client.OnMessagePublic = func(p *dctk.Peer, content string) {
		if content == "hi bot" {
			client.MessagePublic("hello all")
		}
	}

	client.Run()
}
