package main

import (
    "os"
    dctk "github.com/gswly/dctoolkit"
)

var ok = false

func client1() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubUrl: os.Getenv("HUBURL"),
        Nick: "client1",
        ModePassive: true,
    })
    if err != nil {
        panic(err)
    }

    client.OnPrivateMessage = func(p *dctk.Peer, content string) {
        if p.Nick == "client2" && content == "hi client1" {
            client.PrivateMessage(p, "hi client2")
        }
    }

    client.Run()
}

func client2() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubUrl: os.Getenv("HUBURL"),
        Nick: "client2",
        ModePassive: true,
    })
    if err != nil {
        panic(err)
    }

    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == "client1" {
            client.PrivateMessage(p, "hi client1")
        }
    }

    client.OnPrivateMessage = func(p *dctk.Peer, content string) {
        if p.Nick == "client1" && content == "hi client2" {
            ok = true
            client.Terminate()
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
