// +build ignore

package main

import (
	"fmt"
	dctk "github.com/gswly/dctoolkit"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

var ok = false

func client1() {
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:           os.Getenv("HUBURL"),
		Nick:             "client1",
		PrivateIp:        true,
		HubManualConnect: true,
		TcpPort:          3006,
		UdpPort:          3006,
		TcpTlsPort:       3007,
	})
	if err != nil {
		panic(err)
	}

	os.Mkdir("/share", 0755)
	os.Mkdir("/share/inner folder", 0755)
	ioutil.WriteFile("/share/inner folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

	client.OnInitialized = func() {
		client.ShareAdd("aliasname", "/share")
	}

	client.OnShareIndexed = func() {
		client.HubConnect()
	}

	client.Run()
}

func client2() {
	isGodcppNmdc := strings.HasPrefix(os.Getenv("HUBURL"), "nmdc://") &&
		strings.HasSuffix(os.Getenv("HUBURL"), ":1411")
	isAdc := strings.HasPrefix(os.Getenv("HUBURL"), "adc")
	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:     os.Getenv("HUBURL"),
		Nick:       "client2",
		PrivateIp:  true,
		TcpPort:    3005,
		UdpPort:    3005,
		TcpTlsPort: 3004,
	})
	if err != nil {
		panic(err)
	}

	client.OnPeerConnected = func(p *dctk.Peer) {
		if p.Nick == "client1" {
			go func() {
				time.Sleep(1 * time.Second)
				client.Safe(func() {
					client.Search(dctk.SearchConf{
						Type:  dctk.SearchDirectory,
						Query: "ner fo",
					})
				})
			}()
		}
	}

	step := 0
	client.OnSearchResult = func(res *dctk.SearchResult) {
		switch step {
		case 0:
			if res.IsDir != true ||
				res.Path != "/aliasname/inner folder" ||
				res.TTH != nil ||
				// res.Size for folders is provided by ADC, not provided by NMDC
				((!isAdc && res.Size != 0) || (isAdc && res.Size != 10000)) ||
				((!isGodcppNmdc && res.IsActive != true) || (isGodcppNmdc && res.IsActive != false)) {
				panic(fmt.Errorf("wrong result (1): %+v", res))
			}
			step++
			client.Search(dctk.SearchConf{
				Query: "test file",
			})

		case 1:
			if res.IsDir != false ||
				res.Path != "/aliasname/inner folder/test file.txt" ||
				*res.TTH != dctk.TigerHashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
				res.Size != 10000 ||
				((!isGodcppNmdc && res.IsActive != true) || (isGodcppNmdc && res.IsActive != false)) {
				panic(fmt.Errorf("wrong result (2): %+v", res))
			}
			step++
			client.Search(dctk.SearchConf{
				Type: dctk.SearchTTH,
				TTH:  dctk.TigerHashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
			})

		case 2:
			if res.IsDir != false ||
				res.Path != "/aliasname/inner folder/test file.txt" ||
				*res.TTH != dctk.TigerHashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
				res.Size != 10000 ||
				((!isGodcppNmdc && res.IsActive != true) || (isGodcppNmdc && res.IsActive != false)) {
				panic(fmt.Errorf("wrong result (3): %+v", res))
			}
			ok = true
			client.Close()
		}
	}

	client.Run()
}

func main() {
	dctk.SetLogLevel(dctk.LevelDebug)

	go client1()
	client2()

	if ok == false {
		panic("test failed")
	}
}
