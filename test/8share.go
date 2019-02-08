package main

import (
    "time"
    "fmt"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    ok := false
    dctk.SetLogLevel(dctk.LevelDebug)

    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "dctk-verlihub",
        HubPort: 4111,
        HubManualConnect: true,
        Nick: "testdctk",
        PrivateIp: true,
        ModePassive: true,
    })
    if err != nil {
        panic(err)
    }

    client.OnInitialized = func() {
        client.ShareAdd("etc", "/etc")
    }

    reindexed := false
    client.OnShareIndexed = func() {
        fmt.Println("indexed")

        if reindexed == false {
            reindexed = true
            client.HubConnect()

            go func() {
                time.Sleep(2 * time.Second)
                client.Safe(func() {
                    client.ShareAdd("etc", "/etc")
                })
            }()

        } else {
            ok = true
            client.Terminate()
        }
    }

    client.Run()

    if ok == false {
        panic("test failed")
    }
}
