package dctoolkit_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestDownloadOnDisk(t *testing.T) {
	foreachExternalHub(t, func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				HubUrl:           e.Url(),
				Nick:             "client1",
				Ip:               "127.0.0.1",
				TcpPort:          3006,
				UdpPort:          3006,
				TcpTlsPort:       3007,
				HubManualConnect: true,
			})
			require.NoError(t, err)

			os.RemoveAll("/testshare")
			os.Mkdir("/testshare", 0755)
			ioutil.WriteFile("/testshare/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				HubUrl:     e.Url(),
				Nick:       "client2",
				Ip:         "127.0.0.1",
				TcpPort:    3005,
				UdpPort:    3005,
				TcpTlsPort: 3004,
			})
			require.NoError(t, err)

			client.OnPeerConnected = func(p *dctk.Peer) {
				if p.Nick == "client1" {
					client.DownloadFile(dctk.DownloadConf{
						Peer:     p,
						TTH:      dctk.TigerHashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
						SavePath: "/tmp/outfile",
					})
				}
			}

			client.OnDownloadSuccessful = func(d *dctk.Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		dctk.SetLogLevel(dctk.LevelError)

		go client1()
		client2()

		require.True(t, ok)
	})
}
