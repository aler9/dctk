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

	// download peer file list
	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "client" {
			client.DownloadFileList(p, "")
		}
	}

	filelistDownloaded := false
	client.OnDownloadSuccessful = func(d *dctk.Download) {
		// file list has been downloaded
		if filelistDownloaded == false {
			filelistDownloaded = true

			// parse file list
			fl, err := dctk.FileListParse(d.Content())
			if err != nil {
				panic(err)
			}

			// find file
			file, err := fl.GetFile("/path/to/file")
			if err != nil {
				panic(err)
			}

			// start downloading the file
			client.DownloadFLFile(d.Conf().Peer, file, "/tmp/directory")

			// file has been downloaded
		} else {
			client.Close()
		}
	}

	client.Run()
}
