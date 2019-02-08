package main

import (
    "os"
    "strings"
    "io/ioutil"
    dctk "github.com/gswly/dctoolkit"
)

var ok = false

func client1() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "dctk-verlihub",
        HubPort: 4111,
        Nick: "client1",
        PrivateIp: true,
        TcpPort: 3006,
        UdpPort: 3006,
        TcpTlsPort: 3007,
        PeerEncryptionMode: dctk.ForceEncryption,
        HubManualConnect: true,
    })
    if err != nil {
        panic(err)
    }

    os.Mkdir("/share", 0755)
    ioutil.WriteFile("/share/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

    client.OnInitialized = func() {
        client.ShareAdd("share", "/share")
    }

    client.OnShareIndexed = func() {
        client.HubConnect()
    }

    client.Run()
}

func client2() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "dctk-verlihub",
        HubPort: 4111,
        Nick: "client2",
        PrivateIp: true,
        TcpPort: 3005,
        TcpTlsPort: 3004,
        UdpPort: 3005,
        PeerEncryptionMode: dctk.ForceEncryption,
    })
    if err != nil {
        panic(err)
    }

    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == "client1" {
            client.Download(dctk.DownloadConf{
                Nick: "client1",
                TTH: "UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY",
            })
        }
    }

    client.OnDownloadSuccessful = func(d* dctk.Download) {
        ok = true
        client.Terminate()
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
