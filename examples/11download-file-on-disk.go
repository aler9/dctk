// +build ignore

package main

import (
	"github.com/aler9/dctk"
	"github.com/aler9/dctk/pkg/tiger"
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

	// download a file by tth
	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "nickname" {
			client.DownloadFile(dctk.DownloadConf{
				Peer:     p,
				TTH:      tiger.HashMust("AJ64KGNQ7OKNE7O4ARMYNWQ2VJF677BMUUQAR3Y"),
				SavePath: "/path/to/outfile",
			})
		}
	}

	// download has finished: close
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		client.Close()
	}

	client.Run()
}
