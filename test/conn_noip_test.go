package dctoolkit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestConnNoIp(t *testing.T) {
	foreachExternalHub(t, "ConnNoIp", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := dctk.NewClient(dctk.ClientConf{
			LogLevel:   dctk.LogLevelError,
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
