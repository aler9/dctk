// +build ignore

package main

import (
	"github.com/aler9/dctk"
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

	// when we are connected, start downloading the file list of every other peer
	// who share at least one byte of files and is not ourself
	client.OnHubConnected = func() {
		for _, p := range client.Peers() {
			if p.ShareSize > 0 && p.Nick != client.Conf().Nick {
				client.DownloadFileList(p, "")
			}
		}
	}

	// a file list has been downloaded. When there are none remaining, close connection
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		if client.DownloadCount() == 0 {
			client.Close()
		}
	}

	client.Run()
}
