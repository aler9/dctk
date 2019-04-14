// +build ignore

package main

import (
	"fmt"
	dctk "github.com/gswly/dctoolkit"
)

func main() {
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

	// to stream a file you must know the exact file size, peer and TTH,
	// or otherwise start a search like it is done in this example
	var filePeer *dctk.Peer
	fileSize := uint64(0)
	fileTTH := dctk.TigerHash{}
	fileCurPos := uint64(0)
	dlStarted := false
	const chunkLen = uint64(1024 * 1024)

	downloadNextChunk := func() {
		if fileCurPos >= fileSize {
			fmt.Println("download complete.")
			client.Close()
			return
		}

		downloadLen := chunkLen
		if (fileCurPos + downloadLen) > fileSize {
			downloadLen = fileSize - fileCurPos
		}
		client.DownloadFile(dctk.DownloadConf{
			Peer:   filePeer,
			TTH:    fileTTH,
			Start:  fileCurPos,
			Length: int64(downloadLen),
		})
		fileCurPos += downloadLen
	}

	client.OnHubConnected = func() {
		client.Search(dctk.SearchConf{
			Type: dctk.SearchTTH,
			TTH:  dctk.TigerHashMust("myTTH"),
		})
	}

	client.OnSearchResult = func(res *dctk.SearchResult) {
		if !dlStarted {
			dlStarted = true
			filePeer = res.Peer
			fileSize = res.Size
			fileTTH = res.TTH
			downloadNextChunk()
		}
	}

	client.OnDownloadSuccessful = func(d *dctk.Download) {
		fmt.Printf("chunk available: %d\n", len(d.Content()))
		downloadNextChunk()
	}

	client.Run()
}
