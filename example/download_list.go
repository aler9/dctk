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
        TcpPort: 3006,
        TcpTlsPort: 3007,
        UdpPort: 3006,
    })
    if err != nil {
        panic(err)
    }

    client.OnHubConnected = func() {
        // download file list of a certain user
        client.DownloadFileList("othernick")
    }

    client.OnDownloadSuccessful = func(d *dctk.Download) {
        fmt.Println("downloaded: %d", len(d.Content()))
    }

    client.Run()
}
