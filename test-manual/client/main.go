// manual client.
package main

import (
	"github.com/aler9/dctk"
	"github.com/aler9/dctk/pkg/log"
)

func main() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubURL:             "nmdc://127.0.0.1:4111",
		Nick:               "testclient",
		TCPPort:            3009,
		UDPPort:            3009,
		TLSPort:            3010,
		HubManualConnect:   true,
		IsPassive:          true,
		PeerEncryptionMode: dctk.DisableEncryption,
		LogLevel:           log.LevelDebug,
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

	client.Run()
}
