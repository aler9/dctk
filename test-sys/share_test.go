package dctoolkit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestShare(t *testing.T) {
	foreachExternalHub(t, func(t *testing.T, e *externalHub) {
		ok := false
		dctk.SetLogLevel(dctk.LevelError)

		client, err := dctk.NewClient(dctk.ClientConf{
			HubUrl:           e.Url(),
			HubManualConnect: true,
			Nick:             "testdctk",
			Ip:               "127.0.0.1",
			IsPassive:        true,
		})
		require.NoError(t, err)

		client.OnInitialized = func() {
			client.ShareAdd("etc", "/etc")
		}

		reindexed := false
		client.OnShareIndexed = func() {
			if reindexed == false {
				reindexed = true
				client.HubConnect()

				go func() {
					time.Sleep(2 * time.Second)
					client.Safe(func() {
						client.ShareAdd("etc", "/etc")
					})
				}()

			} else {
				ok = true
				client.Close()
			}
		}

		client.Run()
		require.True(t, ok)
	})
}
