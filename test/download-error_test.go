package dctk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk"
	"github.com/aler9/dctk/log"
	"github.com/aler9/dctk/tiger"
)

func TestDownloadError(t *testing.T) {
	foreachExternalHub(t, "DownloadError", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				LogLevel:           log.LevelError,
				HubUrl:             e.Url(),
				Nick:               "client1",
				Ip:                 dockerIp,
				TcpPort:            3006,
				UdpPort:            3006,
				PeerEncryptionMode: dctk.DisableEncryption,
			})
			require.NoError(t, err)

			client.Run()
		}

		client2 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				LogLevel:           log.LevelError,
				HubUrl:             e.Url(),
				Nick:               "client2",
				Ip:                 dockerIp,
				TcpPort:            3005,
				UdpPort:            3005,
				PeerEncryptionMode: dctk.DisableEncryption,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			// request a nonexistent file
			client.OnPeerConnected = func(p *dctk.Peer) {
				if p.Nick == "client1" {
					client.DownloadFile(dctk.DownloadConf{
						Peer: p,
						TTH:  tiger.HashMust("UAUIOGYVALWRB56PRJEB6ZH3G4OLTELOEQ3UKMY"),
					})
				}
			}

			client.OnDownloadError = func(d *dctk.Download) {
				ok = true
				client.Close()
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}
