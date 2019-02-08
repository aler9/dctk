package main

import (
    "io/ioutil"
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
        // download a file by tth
        client.Download(dctk.DownloadConf{
            Nick: "othernick",
            TTH: "AJ64KGNQ7OKNE7O4ARMYNWQ2VJF677BMUUQAR3Y",
        })
    }

    client.OnDownloadSuccessful = func(d *dctk.Download) {
        if err := ioutil.WriteFile("/path/to/outfile", d.Content(), 0655); err != nil {
            panic(err)
        }
    }

    client.Run()
}
