package dctk_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk"
	"github.com/aler9/dctk/log"
)

func TestConnNoIp(t *testing.T) {
	foreachExternalHub(t, "ConnNoIp", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := dctk.NewClient(dctk.ClientConf{
			LogLevel:   log.LevelError,
			HubUrl:     e.Url(),
			Nick:       "testdctk",
			Ip:         dockerIp,
			TcpPort:    3006,
			UdpPort:    3006,
			TcpTlsPort: 3007,
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
