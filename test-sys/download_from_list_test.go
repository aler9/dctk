package dctoolkit_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestDownloadFromList(t *testing.T) {
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
			os.Mkdir("/testshare/folder", 0755)
			ioutil.WriteFile("/testshare/folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0644)

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
					client.DownloadFileList(p, "")
				}
			}

			filelistDownloaded := false
			client.OnDownloadSuccessful = func(d *dctk.Download) {
				if filelistDownloaded == false {
					filelistDownloaded = true

					fl, err := dctk.FileListParse(d.Content())
					require.NoError(t, err)

					file, err := fl.GetFile("/share/folder/test file.txt")
					require.NoError(t, err)

					client.DownloadFLFile(d.Conf().Peer, file, "")

				} else {
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
