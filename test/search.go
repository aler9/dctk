package main

import (
    "os"
    "time"
    "strings"
    "io/ioutil"
    dctk "github.com/gswly/dctoolkit"
)

var ok = false

func client1() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "gotk-verlihub",
        HubPort: 4111,
        Nick: "client1",
        PrivateIp: true,
        HubManualConnect: true,
        TcpPort: 3006,
        TcpTlsPort: 3007,
        UdpPort: 3006,
    })
    if err != nil {
        panic(err)
    }

    os.Mkdir("/share", 0755)
    ioutil.WriteFile("/share/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

    client.OnInitialized = func() {
        client.ShareAdd("aliasname", "/share")
    }

    client.OnShareIndexed = func() {
        client.HubConnect()
    }

    client.Run()
}

func client2() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "gotk-verlihub",
        HubPort: 4111,
        Nick: "client2",
        PrivateIp: true,
        TcpPort: 3005,
        TcpTlsPort: 3004,
        UdpPort: 3005,
    })
    if err != nil {
        panic(err)
    }

    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == "client1" {
            go func() {
                time.Sleep(2 * time.Second)
                client.Safe(func() {
                    client.Search(dctk.SearchConf{
                        Type: dctk.TypeAny,
                        Query: "test file",
                    })
                })
            }()
        }
    }

    searchrecv := false
    client.OnSearchResult = func(res *dctk.SearchResult) {
        if searchrecv == false {
            searchrecv = true
            client.Search(dctk.SearchConf{
                Type: dctk.TypeTTH,
                Query: "UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY",
            })
        } else {
            ok = true
            client.Terminate()
        }
    }

    client.Run()
}

func main() {
    dctk.SetLogLevel(dctk.LevelDebug)

    go client1()
    client2()

    if ok == false {
        panic("test failed")
    }
}
