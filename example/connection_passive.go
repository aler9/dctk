package main

import (
    "fmt"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    // automatically connect to hub
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "hubip",
        HubPort: 411,
        Nick: "mynick",
        Mode: dctk.ModePassive,
    })
    if err != nil {
        panic(err)
    }

    // we are connected to the hub
    client.OnHubConnected = func() {
        fmt.Println("connected to hub")
    }

    client.Run()
}
