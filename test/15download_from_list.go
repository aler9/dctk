// +build ignore

package main

import (
	"io/ioutil"
	"os"
	"strings"

	dctk "github.com/gswly/dctoolkit"
)

var ok = false

func client1() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:           os.Getenv("HUBURL"),
		Nick:             "client1",
		PrivateIp:        true,
		TcpPort:          3006,
		UdpPort:          3006,
		TcpTlsPort:       3007,
		HubManualConnect: true,
	})
	if err != nil {
		panic(err)
	}

	os.Mkdir("/share", 0755)
	os.Mkdir("/share/folder", 0755)
	ioutil.WriteFile("/share/folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

	client.OnInitialized = func() {
		client.ShareAdd("share", "/share")
	}

	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	client.Run()
}

func client2() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:     os.Getenv("HUBURL"),
		Nick:       "client2",
		PrivateIp:  true,
		TcpPort:    3005,
		UdpPort:    3005,
		TcpTlsPort: 3004,
	})
	if err != nil {
		panic(err)
	}

	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "client1" {
			client.DownloadFileList(p, "")
		}
	}

	filelistDownloaded := false
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		if filelistDownloaded == false {
			filelistDownloaded = true

			fl, err := dctk.FileListParse(d.Content())
			if err != nil {
				panic(err)
			}

			file, err := fl.GetFile("/share/folder/test file.txt")
			if err != nil {
				panic(err)
			}

			client.DownloadFLFile(d.Conf().Peer, file, "")

		} else {
			ok = true
			client.Close()
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
