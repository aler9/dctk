package main

import (
    "time"
    dctk "github.com/gswly/dctoolkit"
)

func main() {
    ok := false
    dctk.SetLogLevel(dctk.LevelDebug)

    client,err := dctk.NewClient(dctk.ClientConf{
        HubAddress: "dctk-verlihub",
        HubPort: 4111,
        Nick: "[OP]testdctk",
        Password: "testpa$ss",
        PrivateIp: true,
        TcpPort: 3006,
        TcpTlsPort: 3007,
        UdpPort: 3006,
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
