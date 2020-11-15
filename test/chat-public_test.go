package dctk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk"
	"github.com/aler9/dctk/pkg/log"
)

func TestChatPublic(t *testing.T) {
	foreachExternalHub(t, "ChatPublic", func(t *testing.T, e *externalHub) {
		ok := false

		client1 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				LogLevel:  log.LevelError,
				HubUrl:    e.Url(),
				Nick:      "client1",
				IsPassive: true,
			})
			require.NoError(t, err)

			client.OnMessagePublic = func(p *dctk.Peer, content string) {
				if p.Nick == "client2" {
					if content == "hi client1" {
						client.MessagePublic("hi client2")
					}
				}
			}

			client.Run()
		}

		client2 := func() {
			client, err := dctk.NewClient(dctk.ClientConf{
				LogLevel:  log.LevelError,
				HubUrl:    e.Url(),
				Nick:      "client2",
				IsPassive: true,
			})
			require.NoError(t, err)

			client.OnHubConnected = func() {
				go client1()
			}

			client.OnPeerConnected = func(p *dctk.Peer) {
				if p.Nick == "client1" {
					client.MessagePublic("hi client1")
				}
			}

			client.OnMessagePublic = func(p *dctk.Peer, content string) {
				if p.Nick == "client1" {
					if content == "hi client2" {
						ok = true
						client.Close()
					}
				}
			}

			client.Run()
		}

		client2()

		require.True(t, ok)
	})
}
