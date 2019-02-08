package main

import (
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "hubip",
        HubPort: 411,
        TcpPort: 3006,
        TcpTlsPort: 3007,
        UdpPort: 3006,
    })
    if err != nil {
        panic(err)
    }

    client.OnHubConnected = func() {
        for _,p := range client.Peers() {
            client.DownloadFileList(p.Nick)
        }
    }

    client.OnDownloadSuccessful = func(d *dctk.Download) {
        if client.DownloadCount() == 0 {
            client.Terminate()
        }
    }

    client.Run()
}
