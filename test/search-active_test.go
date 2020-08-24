package dctoolkit_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
	"github.com/aler9/dctoolkit/tiger"
)

func TestSearchActive(t *testing.T) {
	foreachExternalHub(t, "SearchActive", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				LogLevel:         dctk.LogLevelError,
				HubUrl:           e.Url(),
				Nick:             "client1",
				Ip:               dockerIp,
				HubManualConnect: true,
				TcpPort:          3006,
				UdpPort:          3006,
				TcpTlsPort:       3007,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0755)
			os.Mkdir("/tmp/testshare/inner folder", 0755)
			ioutil.WriteFile("/tmp/testshare/inner folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

			client.OnInitialized = func() {
				client.ShareAdd("aliasname", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			isGodcppNmdc := strings.HasPrefix(e.Url(), "nmdc://") &&
				strings.HasSuffix(e.Url(), ":1411")
			isAdc := strings.HasPrefix(e.Url(), "adc")

			client, err := dctk.NewClient(dctk.ClientConf{
				LogLevel:   dctk.LogLevelError,
				HubUrl:     e.Url(),
				Nick:       "client2",
				Ip:         dockerIp,
				TcpPort:    3005,
				UdpPort:    3005,
				TcpTlsPort: 3004,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
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
						t.Errorf("wrong result (1): %+v", res)
					}
					step++
					client.Search(dctk.SearchConf{
						Query: "test file",
					})

				case 1:
					if res.IsDir != false ||
						res.Path != "/aliasname/inner folder/test file.txt" ||
						*res.TTH != tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
						res.Size != 10000 ||
						((!isGodcppNmdc && res.IsActive != true) || (isGodcppNmdc && res.IsActive != false)) {
						t.Errorf("wrong result (2): %+v", res)
					}
					step++
					client.Search(dctk.SearchConf{
						Type: dctk.SearchTTH,
						TTH:  tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})

				case 2:
					if res.IsDir != false ||
						res.Path != "/aliasname/inner folder/test file.txt" ||
						*res.TTH != tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
						res.Size != 10000 ||
						((!isGodcppNmdc && res.IsActive != true) || (isGodcppNmdc && res.IsActive != false)) {
						t.Errorf("wrong result (3): %+v", res)
					}
					ok = true
					client.Close()
				}
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}
