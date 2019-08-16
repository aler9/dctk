// +build ignore

package main

import (
	dctk "github.com/gswly/dctoolkit"
	"os"
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
	dctk.SetLogLevel(dctk.LevelInfo)

	go client1()
	client2()

	if ok == false {
		panic("test failed")
	}
}
