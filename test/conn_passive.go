package main

import (
    "time"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    ok := false
    dctk.SetLogLevel(dctk.LevelDebug)

    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "gotk-verlihub",
        HubPort: 4111,
        Nick: "testdctk",
        ModePassive: true,
    })
    if err != nil {
        panic(err)
    }

    client.OnHubConnected = func() {
        go func() {
            time.Sleep(1 * time.Second)
            client.Safe(func() {
                ok = true
                client.Terminate()
            })
        }()
    }

    client.Run()

    if ok == false {
        panic("test failed")
    }
}
