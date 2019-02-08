package main

import (
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "hubip",
        HubPort: 411,
        Nick: "mynick",
        ModePassive: true,
    })
    if err != nil {
        panic(err)
    }

    client.OnPublicMessage = func(p *dctk.Peer, content string) {
        if content == "hi bot" {
            client.PublicMessage("hello all")
        }
    }

    client.Run()
}
