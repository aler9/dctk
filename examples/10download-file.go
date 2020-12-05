// +build ignore

package main

import (
	"fmt"

	"github.com/aler9/dctk"
	"github.com/aler9/dctk/pkg/tiger"
)

func main() {
	// connect to hub in active mode. local ports must be opened and accessible.
	client, err := dctk.NewClient(dctk.ClientConf{
		HubURL:  "nmdc://hubip:411",
		Nick:    "mynick",
		TCPPort: 3009,
		UDPPort: 3009,
		TLSPort: 3010,
	})
	if err != nil {
		panic(err)
	}

	// download a file by tth, keep it in RAM
	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "nickname" {
			client.DownloadFile(dctk.DownloadConf{
				Peer: p,
				TTH:  tiger.HashMust("AJ64KGNQ7OKNE7O4ARMYNWQ2VJF677BMUUQAR3Y"),
			})
		}
	}

	// download has finished
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		fmt.Printf("downloaded: %d\n", len(d.Content()))
		client.Close()
	}

	client.Run()
}
