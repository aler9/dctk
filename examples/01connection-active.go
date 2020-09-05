// +build ignore

package main

import (
	"fmt"

	"github.com/aler9/dctk"
)

func main() {
	// connect to hub in active mode. ports must be opened and accessible.
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

	// we are connected to the hub
	client.OnHubConnected = func() {
		fmt.Println("connected to hub")
	}

	client.Run()
}
