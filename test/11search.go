// +build ignore

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
        HubUrl: os.Getenv("HUBURL"),
        Nick: "client1",
        PrivateIp: true,
        HubManualConnect: true,
        TcpPort: 3006,
        UdpPort: 3006,
        TcpTlsPort: 3007,
    })
    if err != nil {
        panic(err)
    }

    os.Mkdir("/share", 0755)
    os.Mkdir("/share/inner folder", 0755)
    ioutil.WriteFile("/share/inner folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

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
        HubUrl: os.Getenv("HUBURL"),
        Nick: "client2",
        PrivateIp: true,
        TcpPort: 3005,
        UdpPort: 3005,
        TcpTlsPort: 3004,
    })
    if err != nil {
        panic(err)
    }

    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == "client1" {
            go func() {
                time.Sleep(1 * time.Second)
                client.Safe(func() {
                    client.Search(dctk.SearchConf{
                        Type: dctk.SearchDirectory,
                        Query: "ner fo",
                    })
                })
            }()
        }
    }

    step := 0
    client.OnSearchResult = func(res *dctk.SearchResult) {
        switch step {
        case 0:
            if res.IsDir == true && res.Path == "/aliasname/inner folder" &&
                res.TTH == "" && // res.Size for folders is provided by ADC, not provided by NMDC
                res.IsActive == true {
                step++
                client.Search(dctk.SearchConf{
                    Query: "test file",
                })
            }

        case 1:
            if res.IsDir == false && res.Path == "/aliasname/inner folder/test file.txt" &&
                res.TTH == "UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY" && res.Size == 10000 &&
                res.IsActive == true {
                step++
                client.Search(dctk.SearchConf{
                    Type: dctk.SearchTTH,
                    Query: "UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY",
                })
            }

        case 2:
            if res.IsDir == false && res.Path == "/aliasname/inner folder/test file.txt" &&
                res.TTH == "UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY" && res.Size == 10000 &&
                res.IsActive == true {
                ok = true
                client.Terminate()
            }
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
