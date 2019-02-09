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

    // download has finished
    client.OnDownloadSuccessful = func(d *Download) {
        fmt.Println("file downloaded and available in d.Content()")
        client.Terminate()
    }

    client.Run()
}
