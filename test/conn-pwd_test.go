package dctk_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk"
	"github.com/aler9/dctk/pkg/log"
)

func TestConnPwd(t *testing.T) {
	foreachExternalHub(t, "ConnPwd", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := dctk.NewClient(dctk.ClientConf{
			LogLevel: log.LevelError,
			HubURL:   e.URL(),
			Nick:     "testdctk_auth",
			Password: "testpa$ss",
			IP:       dockerIP,
			TCPPort:  3006,
			UDPPort:  3006,
			TLSPort:  3007,
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
