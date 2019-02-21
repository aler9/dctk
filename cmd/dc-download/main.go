package main

import (
    "path/filepath"
    "gopkg.in/alecthomas/kingpin.v2"
    dctk "github.com/gswly/dctoolkit"
)

var (
    hub = kingpin.Flag("hub", "The url of a hub, ie nmdc://hubip:411").Required().String()
    nick = kingpin.Flag("nick", "The nickname to use").Required().String()
    pwd = kingpin.Flag("pwd", "The password to use").String()
    passive = kingpin.Flag("passive", "Turn on passive mode (ports are not required anymore)").Bool()
    tcpPort = kingpin.Flag("tcp", "The TCP port to use").Default("3009").Uint()
    udpPort = kingpin.Flag("udp", "The UDP port to use").Default("3009").Uint()
    tlsPort = kingpin.Flag("tls", "The TCP-TLS port to use").Default("3010").Uint()
    share = kingpin.Flag("share", "An (optional) folder to share. Some hubs require a minimum share").String()
    outdir = kingpin.Flag("outdir", "The directory in which files will be saved").Required().String()
    user = kingpin.Arg("user", "The user from which to download").Required().String()
    fpath = kingpin.Arg("fpath", "The path of the file or directory to download").Required().String()
)

func main() {
    kingpin.CommandLine.Help = "Download a file or a directory from a user in a given hub."
    kingpin.Parse()

    client,err := dctk.NewClient(dctk.ClientConf{
        HubUrl: *hub,
        Nick: *nick,
        Password: *pwd,
        TcpPort: *tcpPort,
        UdpPort: *udpPort,
        TcpTlsPort: *tlsPort,
        IsPassive: *passive,
        HubManualConnect: true,
    })
    if err != nil {
        panic(err)
    }

    client.OnInitialized = func() {
        if *share != "" {
            client.ShareAdd("share", *share)
        } else {
            client.HubConnect()
        }
    }

    client.OnShareIndexed = func() {
        client.HubConnect()
    }

    client.OnPeerConnected = func(p *dctk.Peer) {
        if p.Nick == *user {
            client.DownloadFileList(p, "")
        }
    }

    filelistDownloaded := false
    client.OnDownloadSuccessful = func(d* dctk.Download) {
        if filelistDownloaded == false {
            filelistDownloaded = true

            fl,err := dctk.FileListParse(d.Content())
            if err != nil {
                panic(err)
            }

            // check if it is a file
            file,err := fl.GetFile(*fpath)
            if err == nil {
                client.DownloadFLFile(d.Conf().Peer, file, filepath.Join(*outdir, file.Name))
                return
            }

            // check if it is a directory
            dir,err := fl.GetDirectory(*fpath)
            if err == nil {
                client.DownloadFLDirectory(d.Conf().Peer, dir, filepath.Join(*outdir, dir.Name))
                return
            }

            panic("file or directory not found")

        } else {
            if client.DownloadCount() == 0 {
                client.Terminate()
            }
        }
    }

    client.Run()
}
