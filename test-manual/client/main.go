package main

import (
	dctk "github.com/aler9/dctoolkit"
)

func main() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:           "nmdc://127.0.0.1:4111",
		Nick:             "testclient",
		TcpPort:          3009,
		UdpPort:          3009,
		TcpTlsPort:       3010,
		HubManualConnect: true,
	})
	if err != nil {
		panic(err)
	}

	client.OnInitialized = func() {
		client.ShareAdd("share", "/etc")
	}

	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	dctk.SetLogLevel(dctk.LevelDebug)
	client.Run()
}
