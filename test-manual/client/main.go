package main

import (
	"github.com/aler9/dctk"
)

func main() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:             "nmdc://127.0.0.1:4111",
		Nick:               "testclient",
		TcpPort:            3009,
		UdpPort:            3009,
		TcpTlsPort:         3010,
		HubManualConnect:   true,
		IsPassive:          true,
		PeerEncryptionMode: dctk.DisableEncryption,
	})
	if err != nil {
		panic(err)
	}

	client.OnInitialized = func() {
		client.ShareAdd("share", "/share")
	}

	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	dctk.SetLogLevel(log.LevelDebug)
	client.Run()
}
