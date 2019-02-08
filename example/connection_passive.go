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
        Password: "mypassword",
        Mode: dctk.ModePassive,
    })
    if err != nil {
        panic(err)
    }

    client.OnHubConnected = func() {
        fmt.Println("connected to hub")
    }

    client.Run()
}
