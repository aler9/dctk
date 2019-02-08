package main

import (
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    // configure hub but do not connect automatically. local ports must be opened and accessible (configure your router)
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "soundofconfusion.no-ip.org",
        HubPort: 411,
        Nick: "mynick",
        TcpPort: 3005,
        TcpTlsPort: 3006,
        UdpPort: 3006,
        HubManualConnect: true,
    })
    if err != nil {
        panic(err)
    }

    // wait initialization and start indexing
    client.OnInitialized = func() {
        client.ShareAdd("share", "/etc")
    }

    // wait indexing and connect to hub
    client.OnShareIndexed = func() {
        client.HubConnect()
    }

    client.Run()
}
