package main

import (
    "fmt"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    // automatically connect to hub
    client,err := dctk.NewClient(dctk.ClientConf{
        HubUrl: "nmdc://hubip:411",
        Nick: "mynick",
        IsPassive: true,
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
