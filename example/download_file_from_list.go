// +build ignore

package main

import (
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

    // download peer file list
    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == "client" {
            client.DownloadFileList(p)
        }
    }

    filelistDownloaded := false
    client.OnDownloadSuccessful = func(d* dctk.Download) {
        // file list has been downloaded
        if filelistDownloaded == false {
            filelistDownloaded = true

            // parse file list
            fl,err := dctk.FileListParse(d.Content())
            if err != nil {
                panic(err)
            }

            // find file
            file,err := fl.GetFile("/path/to/file")
            if err != nil {
                panic(err)
            }

            // start downloading the file
            client.DownloadFLFile(d.Conf().Peer, file, "/tmp/directory")

        // file has been downloaded
        } else {
            client.Terminate()
        }
    }

    client.Run()
}
