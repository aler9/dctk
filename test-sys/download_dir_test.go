package dctoolkit_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestDownloadDir(t *testing.T) {
	foreachExternalHub(t, func(t *testing.T, e *externalHub) {
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
			os.Mkdir("/testshare/folder/subdir", 0755)
			ioutil.WriteFile("/testshare/folder/first file.txt", []byte(strings.Repeat("A", 50000)), 0644)
			ioutil.WriteFile("/testshare/folder/second file.txt", []byte(strings.Repeat("B", 50000)), 0644)
			ioutil.WriteFile("/testshare/folder/third file.txt", []byte(strings.Repeat("C", 50000)), 0644)
			ioutil.WriteFile("/testshare/folder/subdir/fourth file.txt", []byte(strings.Repeat("D", 50000)), 0644)
			ioutil.WriteFile("/testshare/folder/subdir/fifth file.txt", []byte(strings.Repeat("E", 50000)), 0644)

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

			paths := map[dctk.TigerHash]string{
				dctk.TigerHashMust("I3M75IU7XNESOE6ZJ2AGG2J5CQZIBBKYZLBQ5NI"): "/testshare/folder/first file.txt",
				dctk.TigerHashMust("PZBH3XI6AFTZHB2UCG35FDILNVOT6JAELGOX3AA"): "/testshare/folder/second file.txt",
				dctk.TigerHashMust("GMSFH3RI6S3THNCDSM3RHHDY6XKIIQ64VLLZJQI"): "/testshare/folder/third file.txt",
				dctk.TigerHashMust("V6O5IVOZHCSB5FDMU7ZQ7L4XTF6BTCD2SIZEISI"): "/testshare/folder/subdir/fourth file.txt",
				dctk.TigerHashMust("7PYQKBYSMSNOLMQWS2QKCNBQC65RK5VKNOWTCMY"): "/testshare/folder/subdir/fifth file.txt",
			}

			downloaded := map[dctk.TigerHash]bool{
				dctk.TigerHashMust("I3M75IU7XNESOE6ZJ2AGG2J5CQZIBBKYZLBQ5NI"): false,
				dctk.TigerHashMust("PZBH3XI6AFTZHB2UCG35FDILNVOT6JAELGOX3AA"): false,
				dctk.TigerHashMust("GMSFH3RI6S3THNCDSM3RHHDY6XKIIQ64VLLZJQI"): false,
				dctk.TigerHashMust("V6O5IVOZHCSB5FDMU7ZQ7L4XTF6BTCD2SIZEISI"): false,
				dctk.TigerHashMust("7PYQKBYSMSNOLMQWS2QKCNBQC65RK5VKNOWTCMY"): false,
			}

			filelistDownloaded := false
			client.OnDownloadSuccessful = func(d *dctk.Download) {
				if filelistDownloaded == false {
					filelistDownloaded = true

					fl, err := dctk.FileListParse(d.Content())
					require.NoError(t, err)

					dir, err := fl.GetDirectory("/share/folder")
					require.NoError(t, err)

					client.DownloadFLDirectory(d.Conf().Peer, dir, "/tmp/out")

				} else {
					if _, ok := downloaded[d.Conf().TTH]; !ok {
						t.Errorf("wrong TTH")
					}

					if downloaded[d.Conf().TTH] == true {
						t.Errorf("TTH already downloaded")
					}
					downloaded[d.Conf().TTH] = true

					tth, err := dctk.TTHFromFile(paths[d.Conf().TTH])
					require.NoError(t, err)

					require.Equal(t, tth, d.Conf().TTH)

					if client.DownloadCount() == 0 {
						client.Close()
					}
				}
			}

			client.Run()

			for _, b := range downloaded {
				require.True(t, b)
			}
		}

		dctk.SetLogLevel(dctk.LevelError)

		go client1()
		client2()
	})
}
