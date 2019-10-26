package dctoolkit_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestSearchPassive(t *testing.T) {
	foreachExternalHub(t, func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				HubUrl:           e.Url(),
				Nick:             "client1",
				Ip:               "127.0.0.1",
				HubManualConnect: true,
				TcpPort:          3006,
				UdpPort:          3006,
				TcpTlsPort:       3007,
			})
			require.NoError(t, err)

			os.RemoveAll("/testshare")
			os.Mkdir("/testshare", 0755)
			os.Mkdir("/testshare/inner folder", 0755)
			ioutil.WriteFile("/testshare/inner folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

			client.OnInitialized = func() {
				client.ShareAdd("aliasname", "/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			isAdc := strings.HasPrefix(e.Url(), "adc")
			client, err := dctk.NewClient(dctk.ClientConf{
				HubUrl:    e.Url(),
				Nick:      "client2",
				Ip:        "127.0.0.1",
				IsPassive: true,
			})
			require.NoError(t, err)

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
						res.IsActive != false {
						t.Errorf("wrong result (1): %+v", res)
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
						res.IsActive != false {
						t.Errorf("wrong result (2): %+v", res)
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
						res.IsActive != false {
						t.Errorf("wrong result (3): %+v", res)
					}
					ok = true
					client.Close()
				}
			}

			client.Run()
		}

		dctk.SetLogLevel(dctk.LevelError)

		go client1()
		client2()

		require.True(t, ok)
	})
}
