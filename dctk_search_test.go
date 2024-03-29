package dctk

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/tiger"
)

func TestSearchActive(t *testing.T) {
	foreachExternalHub(t, "SearchActive", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:         log.LevelError,
				HubURL:           e.URL(),
				Nick:             "client1",
				IP:               dockerIP,
				HubManualConnect: true,
				TCPPort:          3006,
				UDPPort:          3006,
				TLSPort:          3007,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			os.Mkdir("/tmp/testshare/inner folder", 0o755)
			os.WriteFile("/tmp/testshare/inner folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("aliasname", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			isGodcppNmdc := strings.HasPrefix(e.URL(), "nmdc://") &&
				strings.HasSuffix(e.URL(), ":1411")
			isAdc := strings.HasPrefix(e.URL(), "adc")

			client, err := NewClient(ClientConf{
				LogLevel: log.LevelError,
				HubURL:   e.URL(),
				Nick:     "client2",
				IP:       dockerIP,
				TCPPort:  3005,
				UDPPort:  3005,
				TLSPort:  3004,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			client.OnPeerConnected = func(p *Peer) {
				if p.Nick == "client1" {
					go func() {
						time.Sleep(1 * time.Second)
						client.Safe(func() {
							client.Search(SearchConf{
								Type:  SearchDirectory,
								Query: "ner fo",
							})
						})
					}()
				}
			}

			step := 0
			client.OnSearchResult = func(res *SearchResult) {
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
					require.NoError(t, client.Search(SearchConf{
						Query: "test file",
					}))

				case 1:
					if res.IsDir != false ||
						res.Path != "/aliasname/inner folder/test file.txt" ||
						*res.TTH != tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
						res.Size != 10000 ||
						((!isGodcppNmdc && res.IsActive != true) || (isGodcppNmdc && res.IsActive != false)) {
						t.Errorf("wrong result (2): %+v", res)
					}
					step++
					client.Search(SearchConf{
						Type: SearchTTH,
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

func TestSearchPassive(t *testing.T) {
	foreachExternalHub(t, "SearchPassive", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:         log.LevelError,
				HubURL:           e.URL(),
				Nick:             "client1",
				IP:               dockerIP,
				HubManualConnect: true,
				TCPPort:          3006,
				UDPPort:          3006,
				TLSPort:          3007,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			os.Mkdir("/tmp/testshare/inner folder", 0o755)
			os.WriteFile("/tmp/testshare/inner folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("aliasname", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			isAdc := strings.HasPrefix(e.URL(), "adc")
			client, err := NewClient(ClientConf{
				LogLevel:  log.LevelError,
				HubURL:    e.URL(),
				Nick:      "client2",
				IP:        dockerIP,
				IsPassive: true,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			client.OnPeerConnected = func(p *Peer) {
				if p.Nick == "client1" {
					go func() {
						time.Sleep(1 * time.Second)
						client.Safe(func() {
							client.Search(SearchConf{
								Type:  SearchDirectory,
								Query: "ner fo",
							})
						})
					}()
				}
			}

			step := 0
			client.OnSearchResult = func(res *SearchResult) {
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
					client.Search(SearchConf{
						Query: "test file",
					})

				case 1:
					if res.IsDir != false ||
						res.Path != "/aliasname/inner folder/test file.txt" ||
						*res.TTH != tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
						res.Size != 10000 ||
						res.IsActive != false {
						t.Errorf("wrong result (2): %+v", res)
					}
					step++
					client.Search(SearchConf{
						Type: SearchTTH,
						TTH:  tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})

				case 2:
					if res.IsDir != false ||
						res.Path != "/aliasname/inner folder/test file.txt" ||
						*res.TTH != tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY") ||
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

		client2()

		require.True(t, ok)
	})
}
