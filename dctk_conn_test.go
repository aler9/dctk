package dctk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk/pkg/log"
)

func TestConnActive(t *testing.T) {
	foreachExternalHub(t, "ConnActive", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := NewClient(ClientConf{
			LogLevel: log.LevelError,
			HubURL:   e.URL(),
			Nick:     "testdctk",
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

func TestConnCompression(t *testing.T) {
	foreachExternalHub(t, "ConnCompression", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := NewClient(ClientConf{
			LogLevel:  log.LevelError,
			HubURL:    e.URL(),
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

func TestConnNoIP(t *testing.T) {
	foreachExternalHub(t, "ConnNoIP", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := NewClient(ClientConf{
			LogLevel: log.LevelError,
			HubURL:   e.URL(),
			Nick:     "testdctk",
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

func TestConnPassive(t *testing.T) {
	foreachExternalHub(t, "ConnPassive", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := NewClient(ClientConf{
			LogLevel:  log.LevelError,
			HubURL:    e.URL(),
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

func TestConnPwd(t *testing.T) {
	foreachExternalHub(t, "ConnPwd", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := NewClient(ClientConf{
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
