package main

import (
	"fmt"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/aler9/dctk"
)

var (
	hub     = kingpin.Flag("hub", "The url of a hub, ie nmdc://hubip:411").Required().String()
	nick    = kingpin.Flag("nick", "The nickname to use").Required().String()
	pwd     = kingpin.Flag("pwd", "The password to use").String()
	passive = kingpin.Flag("passive", "Turn on passive mode (ports are not required anymore)").Bool()
	tcpPort = kingpin.Flag("tcp", "The TCP port to use").Default("3009").Uint()
	udpPort = kingpin.Flag("udp", "The UDP port to use").Default("3009").Uint()
	tlsPort = kingpin.Flag("tls", "The TCP-TLS port to use").Default("3010").Uint()
	share   = kingpin.Flag("share", "An (optional) directory to share. Some hubs require a minimum share").String()
	query   = kingpin.Arg("query", "Search query").Required().String()
)

func main() {
	kingpin.CommandLine.Help = "Search files and directories by name in a given hub."
	kingpin.Parse()

	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:           *hub,
		Nick:             *nick,
		Password:         *pwd,
		TcpPort:          *tcpPort,
		UdpPort:          *udpPort,
		TcpTlsPort:       *tlsPort,
		IsPassive:        *passive,
		HubManualConnect: true,
	})
	if err != nil {
		panic(err)
	}

	client.OnInitialized = func() {
		if *share != "" {
			client.ShareAdd("share", *share)
		} else {
			client.HubConnect()
		}
	}

	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	client.OnHubConnected = func() {
		client.Search(dctk.SearchConf{
			Query: *query,
		})
	}

	client.OnSearchResult = func(r *dctk.SearchResult) {
		fmt.Printf("result: %+v\n", r)
	}

	client.Run()
}
