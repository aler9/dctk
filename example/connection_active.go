package main

import (
    "fmt"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    // automatically connect to hub. local ports must be opened and accessible (configure your router)
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "hubip",
        HubPort: 411,
        Nick: "mynick",
        Password: "mypassword",
        TcpPort: 3006,
        TcpTlsPort: 3007,
        UdpPort: 3006,
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
