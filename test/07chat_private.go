// +build ignore

package main

import (
	"os"

	dctk "github.com/gswly/dctoolkit"
)

var ok = false

func client1() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:    os.Getenv("HUBURL"),
		Nick:      "client1",
		IsPassive: true,
	})
	if err != nil {
		panic(err)
	}

	client.OnMessagePrivate = func(p *dctk.Peer, content string) {
		if p.Nick == "client2" {
			if content == "hi client1" {
				client.MessagePrivate(p, "hi client2")
			}
		}
	}

	client.Run()
}

func client2() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:    os.Getenv("HUBURL"),
		Nick:      "client2",
		IsPassive: true,
	})
	if err != nil {
		panic(err)
	}

	client.OnHubConnected = func() {
		// launch client 2 after client 1 connects, in order to avoid
		// a possible freeze issue with godcpp_adc
		go client1()
	}

	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "client1" {
			client.MessagePrivate(p, "hi client1")
		}
	}

	client.OnMessagePrivate = func(p *dctk.Peer, content string) {
		if p.Nick == "client1" {
			if content == "hi client2" {
				ok = true
				client.Close()
			}
		}
	}

	client.Run()
}

func main() {
	dctk.SetLogLevel(dctk.LevelDebug)

	client2()

	if ok == false {
		panic("test failed")
	}
}
