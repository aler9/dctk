package main

import (
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

    // when we are connected, start downloading the file list of every other peer
    // who share at least one byte of files and is not ourself
    client.OnHubConnected = func() {
        for _,p := range client.Peers() {
            if p.ShareSize > 0 && p.Nick != client.Conf().Nick {
                client.DownloadFileList(p)
            }
        }
    }

    // a file list has been downloaded. When there are none remaining, close connection
    client.OnDownloadSuccessful = func(d *dctk.Download) {
        if client.DownloadCount() == 0 {
            client.Terminate()
        }
    }

    client.Run()
}
