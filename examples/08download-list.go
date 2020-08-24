// +build ignore

package main

import (
	"fmt"

	dctk "github.com/aler9/dctoolkit"
)

func main() {
	// connect to hub in active mode. local ports must be opened and accessible.
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:     "nmdc://hubip:411",
		Nick:       "mynick",
		TcpPort:    3009,
		UdpPort:    3009,
		TcpTlsPort: 3010,
	})
	if err != nil {
		panic(err)
	}

	// download file list of a certain user
	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "nickname" {
			client.DownloadFileList(p, "")
		}
	}

	// download has finished
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		fmt.Printf("downloaded: %d\n", len(d.Content()))
		client.Close()
	}

	client.Run()
}
