package main

import (
	dctk "github.com/aler9/dctoolkit"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	hub     = kingpin.Flag("hub", "The url of a hub, ie nmdc://hubip:411").Required().String()
	nick    = kingpin.Flag("nick", "The nickname to use").Required().String()
	pwd     = kingpin.Flag("pwd", "The password to use").String()
	passive = kingpin.Flag("passive", "Turn on passive mode (ports are not required anymore)").Bool()
	tcpPort = kingpin.Flag("tcp", "The TCP port to use").Default("3009").Uint()
	udpPort = kingpin.Flag("udp", "The UDP port to use").Default("3009").Uint()
	tlsPort = kingpin.Flag("tls", "The TCP-TLS port to use").Default("3010").Uint()
	alias   = kingpin.Flag("alias", "The alias of the share").Default("share").String()
	share   = kingpin.Arg("share", "The directory to share").Required().String()
)

func main() {
	kingpin.CommandLine.Help = "Share a directory in a given hub."
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
		client.ShareAdd(*alias, *share)
	}

	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	client.Run()
}
