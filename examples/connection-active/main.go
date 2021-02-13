package main

import (
	"fmt"

	"github.com/aler9/dctk"
)

func main() {
	// connect to hub in active mode. ports must be opened and accessible.
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

	// we are connected to the hub
	client.OnHubConnected = func() {
		fmt.Println("connected to hub")
	}

	client.Run()
}
