package dctk

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/tiger"
)

func TestDownloadActive(t *testing.T) {
	foreachExternalHub(t, "DownloadActive", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client1",
				IP:                 dockerIP,
				TCPPort:            3006,
				UDPPort:            3006,
				PeerEncryptionMode: DisableEncryption,
				HubManualConnect:   true,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			ioutil.WriteFile("/tmp/testshare/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client2",
				IP:                 dockerIP,
				TCPPort:            3005,
				UDPPort:            3005,
				PeerEncryptionMode: DisableEncryption,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			client.OnPeerConnected = func(p *Peer) {
				if p.Nick == "client1" {
					client.DownloadFile(DownloadConf{
						Peer: p,
						TTH:  tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})
				}
			}

			client.OnDownloadSuccessful = func(d *Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}

func TestDownloadDir(t *testing.T) {
	foreachExternalHub(t, "DownloadDir", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:         log.LevelError,
				HubURL:           e.URL(),
				Nick:             "client1",
				IP:               dockerIP,
				TCPPort:          3006,
				UDPPort:          3006,
				TLSPort:          3007,
				HubManualConnect: true,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			os.Mkdir("/tmp/testshare/folder", 0o755)
			os.Mkdir("/tmp/testshare/folder/subdir", 0o755)
			ioutil.WriteFile("/tmp/testshare/folder/first file.txt", []byte(strings.Repeat("A", 50000)), 0o644)
			ioutil.WriteFile("/tmp/testshare/folder/second file.txt", []byte(strings.Repeat("B", 50000)), 0o644)
			ioutil.WriteFile("/tmp/testshare/folder/third file.txt", []byte(strings.Repeat("C", 50000)), 0o644)
			ioutil.WriteFile("/tmp/testshare/folder/subdir/fourth file.txt", []byte(strings.Repeat("D", 50000)), 0o644)
			ioutil.WriteFile("/tmp/testshare/folder/subdir/fifth file.txt", []byte(strings.Repeat("E", 50000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
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
					client.DownloadFileList(p, "")
				}
			}

			paths := map[tiger.Hash]string{
				tiger.HashMust("I3M75IU7XNESOE6ZJ2AGG2J5CQZIBBKYZLBQ5NI"): "/tmp/testshare/folder/first file.txt",
				tiger.HashMust("PZBH3XI6AFTZHB2UCG35FDILNVOT6JAELGOX3AA"): "/tmp/testshare/folder/second file.txt",
				tiger.HashMust("GMSFH3RI6S3THNCDSM3RHHDY6XKIIQ64VLLZJQI"): "/tmp/testshare/folder/third file.txt",
				tiger.HashMust("V6O5IVOZHCSB5FDMU7ZQ7L4XTF6BTCD2SIZEISI"): "/tmp/testshare/folder/subdir/fourth file.txt",
				tiger.HashMust("7PYQKBYSMSNOLMQWS2QKCNBQC65RK5VKNOWTCMY"): "/tmp/testshare/folder/subdir/fifth file.txt",
			}

			downloaded := map[tiger.Hash]bool{
				tiger.HashMust("I3M75IU7XNESOE6ZJ2AGG2J5CQZIBBKYZLBQ5NI"): false,
				tiger.HashMust("PZBH3XI6AFTZHB2UCG35FDILNVOT6JAELGOX3AA"): false,
				tiger.HashMust("GMSFH3RI6S3THNCDSM3RHHDY6XKIIQ64VLLZJQI"): false,
				tiger.HashMust("V6O5IVOZHCSB5FDMU7ZQ7L4XTF6BTCD2SIZEISI"): false,
				tiger.HashMust("7PYQKBYSMSNOLMQWS2QKCNBQC65RK5VKNOWTCMY"): false,
			}

			filelistDownloaded := false
			client.OnDownloadSuccessful = func(d *Download) {
				if filelistDownloaded == false {
					filelistDownloaded = true

					fl, err := FileListParse(d.Content())
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

					tth, err := tiger.HashFromFile(paths[d.Conf().TTH])
					require.NoError(t, err)

					require.Equal(t, tth, d.Conf().TTH)

					if client.DownloadCount() == 0 {
						ok = true
						client.Close()
					}
				}
			}

			client.Run()

			for _, b := range downloaded {
				require.True(t, b)
			}
		}

		client2()

		require.True(t, ok)
	})
}

func TestDownloadError(t *testing.T) {
	foreachExternalHub(t, "DownloadError", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client1",
				IP:                 dockerIP,
				TCPPort:            3006,
				UDPPort:            3006,
				PeerEncryptionMode: DisableEncryption,
			})
			require.NoError(t, err)

			client.Run()
		}

		client2 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client2",
				IP:                 dockerIP,
				TCPPort:            3005,
				UDPPort:            3005,
				PeerEncryptionMode: DisableEncryption,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			// request a nonexistent file
			client.OnPeerConnected = func(p *Peer) {
				if p.Nick == "client1" {
					client.DownloadFile(DownloadConf{
						Peer: p,
						TTH:  tiger.HashMust("UAUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})
				}
			}

			client.OnDownloadError = func(d *Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}

func TestDownloadFromList(t *testing.T) {
	foreachExternalHub(t, "DownloadFromList", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:         log.LevelError,
				HubURL:           e.URL(),
				Nick:             "client1",
				IP:               dockerIP,
				TCPPort:          3006,
				UDPPort:          3006,
				TLSPort:          3007,
				HubManualConnect: true,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			os.Mkdir("/tmp/testshare/folder", 0o755)
			ioutil.WriteFile("/tmp/testshare/folder/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
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
					client.DownloadFileList(p, "")
				}
			}

			filelistDownloaded := false
			client.OnDownloadSuccessful = func(d *Download) {
				if filelistDownloaded == false {
					filelistDownloaded = true

					fl, err := FileListParse(d.Content())
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

		client2()

		require.True(t, ok)
	})
}

func TestDownloadOnDisk(t *testing.T) {
	foreachExternalHub(t, "DownloadOnDisk", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:         log.LevelError,
				HubURL:           e.URL(),
				Nick:             "client1",
				IP:               dockerIP,
				TCPPort:          3006,
				UDPPort:          3006,
				TLSPort:          3007,
				HubManualConnect: true,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			ioutil.WriteFile("/tmp/testshare/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
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
					client.DownloadFile(DownloadConf{
						Peer:     p,
						TTH:      tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
						SavePath: "/tmp/outfile",
					})
				}
			}

			client.OnDownloadSuccessful = func(d *Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}

func TestDownloadPassive(t *testing.T) {
	foreachExternalHub(t, "DownloadPassive", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client1",
				IP:                 dockerIP,
				TCPPort:            3006,
				UDPPort:            3006,
				PeerEncryptionMode: DisableEncryption,
				HubManualConnect:   true,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			ioutil.WriteFile("/tmp/testshare/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client2",
				IP:                 dockerIP,
				IsPassive:          true,
				PeerEncryptionMode: DisableEncryption,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			client.OnPeerConnected = func(p *Peer) {
				if p.Nick == "client1" {
					client.DownloadFile(DownloadConf{
						Peer: p,
						TTH:  tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})
				}
			}

			client.OnDownloadSuccessful = func(d *Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}

func TestDownloadTls(t *testing.T) {
	foreachExternalHub(t, "DownloadTls", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client1",
				IP:                 dockerIP,
				TCPPort:            3006,
				UDPPort:            3006,
				TLSPort:            3007,
				PeerEncryptionMode: ForceEncryption,
				HubManualConnect:   true,
			})
			require.NoError(t, err)

			os.RemoveAll("/tmp/testshare")
			os.Mkdir("/tmp/testshare", 0o755)
			ioutil.WriteFile("/tmp/testshare/test file.txt", []byte(strings.Repeat("A", 10000)), 0o644)

			client.OnInitialized = func() {
				client.ShareAdd("share", "/tmp/testshare")
			}

			client.OnShareIndexed = func() {
				client.HubConnect()
			}

			client.Run()
		}

		client2 := func() {
			client, err := NewClient(ClientConf{
				LogLevel:           log.LevelError,
				HubURL:             e.URL(),
				Nick:               "client2",
				IP:                 dockerIP,
				TCPPort:            3005,
				UDPPort:            3005,
				TLSPort:            3004,
				PeerEncryptionMode: ForceEncryption,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			client.OnPeerConnected = func(p *Peer) {
				if p.Nick == "client1" {
					client.DownloadFile(DownloadConf{
						Peer: p,
						TTH:  tiger.HashMust("UJUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})
				}
			}

			client.OnDownloadSuccessful = func(d *Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}
