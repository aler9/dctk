package main

import (
    "fmt"
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

    client.OnPrivateMessage = func(p *dctk.Peer, content string) {
        client.PrivateMessage(p, fmt.Sprintf("message received! (%s)", content))
    }

    client.Run()
}
