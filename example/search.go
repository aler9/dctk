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

    client.OnSearchResult = func(r *dctk.SearchResult) {
        fmt.Printf("result: %+v\n", r)
    }

    client.Run()
}
