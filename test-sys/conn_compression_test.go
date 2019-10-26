package dctoolkit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestConnCompression(t *testing.T) {
	foreachExternalHub(t, func(t *testing.T, e *externalHub) {
		ok := false
		dctk.SetLogLevel(dctk.LevelError)

		client, err := dctk.NewClient(dctk.ClientConf{
			HubUrl:    e.Url(),
			Nick:      "testdctk",
			IsPassive: true,
		})
		require.NoError(t, err)

		client.OnHubConnected = func() {
			go func() {
				time.Sleep(1 * time.Second)
				client.Safe(func() {
					ok = true
					client.Close()
				})
			}()
		}

		client.Run()
		require.True(t, ok)
	})
}
