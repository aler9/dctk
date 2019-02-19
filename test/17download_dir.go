// +build ignore

package main

import (
    "fmt"
    "os"
    "strings"
    "io/ioutil"
    dctk "github.com/gswly/dctoolkit"
)

func client1() {
    client,err := dctk.NewClient(dctk.ClientConf{
        HubUrl: os.Getenv("HUBURL"),
        Nick: "client1",
        PrivateIp: true,
        TcpPort: 3006,
        UdpPort: 3006,
        TcpTlsPort: 3007,
        HubManualConnect: true,
    })
    if err != nil {
        panic(err)
    }

    os.Mkdir("/share", 0755)
    os.Mkdir("/share/folder", 0755)
    os.Mkdir("/share/folder/subdir", 0755)
    ioutil.WriteFile("/share/folder/first file.txt", []byte(strings.Repeat("A", 50000)), 0644)
    ioutil.WriteFile("/share/folder/second file.txt", []byte(strings.Repeat("B", 50000)), 0644)
    ioutil.WriteFile("/share/folder/third file.txt", []byte(strings.Repeat("C", 50000)), 0644)
    ioutil.WriteFile("/share/folder/subdir/fourth file.txt", []byte(strings.Repeat("D", 50000)), 0644)
    ioutil.WriteFile("/share/folder/subdir/fifth file.txt", []byte(strings.Repeat("E", 50000)), 0644)

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
            client.DownloadFileList(p)
        }
    }

    paths := map[string]string {
        "I3M75IU7XNESOE6ZJ2AGG2J5CQZIBBKYZLBQ5NI": "/share/folder/first file.txt",
        "PZBH3XI6AFTZHB2UCG35FDILNVOT6JAELGOX3AA": "/share/folder/second file.txt",
        "GMSFH3RI6S3THNCDSM3RHHDY6XKIIQ64VLLZJQI": "/share/folder/third file.txt",
        "V6O5IVOZHCSB5FDMU7ZQ7L4XTF6BTCD2SIZEISI": "/share/folder/subdir/fourth file.txt",
        "7PYQKBYSMSNOLMQWS2QKCNBQC65RK5VKNOWTCMY": "/share/folder/subdir/fifth file.txt",
    }

    downloaded := map[string]bool {
        "I3M75IU7XNESOE6ZJ2AGG2J5CQZIBBKYZLBQ5NI": false,
        "PZBH3XI6AFTZHB2UCG35FDILNVOT6JAELGOX3AA": false,
        "GMSFH3RI6S3THNCDSM3RHHDY6XKIIQ64VLLZJQI": false,
        "V6O5IVOZHCSB5FDMU7ZQ7L4XTF6BTCD2SIZEISI": false,
        "7PYQKBYSMSNOLMQWS2QKCNBQC65RK5VKNOWTCMY": false,
    }

    count := 0
    filelistDownloaded := false
    client.OnDownloadSuccessful = func(d* dctk.Download) {
        if filelistDownloaded == false {
            filelistDownloaded = true

            fl,err := dctk.FileListParse(d.Content())
            if err != nil {
                panic(err)
            }

            dir,err := fl.GetDirectory("/share/folder")
            if err != nil {
                panic(err)
            }

            client.DownloadFLDirectory(d.Conf().Peer, dir, "/tmp/out")

        } else {
            if _,ok := downloaded[d.Conf().TTH]; !ok {
                panic("wrong TTH")
            }

            if downloaded[d.Conf().TTH] == true {
                panic("TTH already downloaded")
            }
            downloaded[d.Conf().TTH] = true

            tth,err := dctk.TTHFromFile(paths[d.Conf().TTH])
            if err != nil {
                panic(err)
            }

            if tth != d.Conf().TTH {
                panic(fmt.Errorf("TTH of file is wrong (%s) (%s)", paths[d.Conf().TTH], tth))
            }

            count++
            fmt.Printf("COUNT: %d\n", count)

            if client.DownloadCount() == 0 {
                client.Terminate()
            }
        }
    }

    client.Run()

    for _,b := range downloaded {
        if b == false {
            panic("file not downloaded")
        }
    }
}

func main() {
    dctk.SetLogLevel(dctk.LevelDebug)

    go client1()
    client2()
}
