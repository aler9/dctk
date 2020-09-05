// +build ignore

package main

import (
	"fmt"

	"github.com/aler9/dctk"
)

func main() {
	// connect to hub in passive mode
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:    "nmdc://hubip:411",
		Nick:      "mynick",
		IsPassive: true,
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
