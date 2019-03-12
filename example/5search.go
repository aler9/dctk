// +build ignore

package main

import (
	"fmt"
	dctk "github.com/gswly/dctoolkit"
)

func main() {
	// connect to hub in active mode. local ports must be opened and accessible.
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

	// hub is connected, start searching
	client.OnHubConnected = func() {
		// search by name
		client.Search(dctk.SearchConf{
			Query: "test",
		})
	}

	// a search result has been received
	client.OnSearchResult = func(r *dctk.SearchResult) {
		fmt.Printf("result: %+v\n", r)
	}

	client.Run()
}
