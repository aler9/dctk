package main

import (
    "fmt"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    // automatically connect to hub. local ports must be opened and accessible (configure your router)
    client,err := dctk.NewClient(dctk.ClientConf{
        HubUrl: "nmdc://hubip:411",
        Nick: "mynick",
        TcpPort: 3009,
        UdpPort: 3009,
        TcpTlsPort: 3010,
    })
    if err != nil {
        panic(err)
    }

    // download file list of a certain user
    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == "nickname" {
            client.DownloadFileList(p)
        }
    }

    // download has finished
    client.OnDownloadSuccessful = func(d *dctk.Download) {
        fmt.Println("downloaded: %d", len(d.Content()))
        client.Terminate()
    }

    client.Run()
}
