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

    // search file by name
    client.OnHubConnected = func() {
        client.Search(dctk.SearchConf{
            Query: "ubuntu",
        })
    }

    // download first result found
    downloadStarted := false
    client.OnSearchResult = func(res *dctk.SearchResult) {
        if downloadStarted == false {
            downloadStarted = true
            client.Download(dctk.DownloadConf{
                Nick: res.Nick,
                TTH: res.TTH,
            })
        }
    }

    client.OnDownloadSuccessful = func(d *Download) {
        fmt.Println("file downloaded and available in d.Content()")
        client.Terminate()
    }

    client.Run()
}
