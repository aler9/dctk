// +build ignore

package main

import (
	dctk "github.com/gswly/dctoolkit"
	"os"
)

var ok = false

func client1() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:             os.Getenv("HUBURL"),
		Nick:               "client1",
		PrivateIp:          true,
		TcpPort:            3006,
		UdpPort:            3006,
		PeerEncryptionMode: dctk.DisableEncryption,
	})
	if err != nil {
		panic(err)
	}

	client.Run()
}

func client2() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:             os.Getenv("HUBURL"),
		Nick:               "client2",
		PrivateIp:          true,
		TcpPort:            3005,
		UdpPort:            3005,
		PeerEncryptionMode: dctk.DisableEncryption,
	})
	if err != nil {
		panic(err)
	}

    // request a nonexistent file
	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "client1" {
			client.DownloadFile(dctk.DownloadConf{
				Peer: p,
				TTH:  dctk.TTHMust("UAUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
			})
		}
	}

	client.OnDownloadError = func(d *dctk.Download) {
		ok = true
		client.Close()
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
