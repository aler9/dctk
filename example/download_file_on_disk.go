// +build ignore

package main

import (
	dctk "github.com/gswly/dctoolkit"
)

func main() {
	// automatically connect to hub. local ports must be opened and accessible (configure your router)
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
				TTH:      dctk.TTHMust("AJ64KGNQ7OKNE7O4ARMYNWQ2VJF677BMUUQAR3Y"),
				SavePath: "/path/to/outfile",
			})
		}
	}

	// download has finished: close
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		client.Terminate()
	}

	client.Run()
}
