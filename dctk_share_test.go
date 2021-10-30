package dctk

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/dctk/pkg/log"
)

func TestShare(t *testing.T) {
	foreachExternalHub(t, "Share", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := NewClient(ClientConf{
			LogLevel:         log.LevelError,
			HubURL:           e.URL(),
			HubManualConnect: true,
			Nick:             "testdctk",
			IP:               dockerIP,
			IsPassive:        true,
		})
		require.NoError(t, err)

		os.RemoveAll("/tmp/testshare")
		os.Mkdir("/tmp/testshare", 0o755)
		os.Mkdir("/tmp/testshare/folder", 0o755)
		ioutil.WriteFile("/tmp/testshare/folder/first file.txt", []byte(strings.Repeat("A", 50000)), 0o644)

		client.OnInitialized = func() {
			client.ShareAdd("share", "/tmp/testshare")
		}

		reindexed := false
		client.OnShareIndexed = func() {
			if reindexed == false {
				reindexed = true
				client.HubConnect()

				go func() {
					time.Sleep(2 * time.Second)
					client.Safe(func() {
						client.ShareAdd("share", "/tmp/testshare")
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
