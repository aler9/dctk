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
        TcpPort: 3009,
        UdpPort: 3009,
        TcpTlsPort: 3010,
    })
    if err != nil {
        panic(err)
    }

    // when hub is connected, start searching
    client.OnHubConnected = func() {
        // search by name
        client.Search(dctk.SearchConf{
            Query: "ubuntu",
        })

        // or search by TTH
        client.Search(dctk.SearchConf{
            Type: dctk.TypeTTH,
            Query: "LDGE7FZYHQUKVFMEBAIMEFLNEMACI5ZGOTZNOIQ",
        })
    }

    // a search result has been received
    client.OnSearchResult = func(r *dctk.SearchResult) {
        fmt.Printf("result: %+v\n", r)
    }

    client.Run()
}
