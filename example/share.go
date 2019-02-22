// +build ignore

package main

import (
	dctk "github.com/gswly/dctoolkit"
)

func main() {
	// configure hub but do not connect automatically. local ports must be opened and accessible (configure your router)
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:           "nmdc://hubip:411",
		Nick:             "mynick",
		TcpPort:          3009,
		UdpPort:          3009,
		TcpTlsPort:       3010,
		HubManualConnect: true,
	})
	if err != nil {
		panic(err)
	}

	// wait initialization and start indexing
	client.OnInitialized = func() {
		client.ShareAdd("share", "/etc")
	}

	// wait indexing and connect to hub
	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	client.Run()
}
