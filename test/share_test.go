package dctoolkit_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dctk "github.com/aler9/dctoolkit"
)

func TestShare(t *testing.T) {
	foreachExternalHub(t, "Share", func(t *testing.T, e *externalHub) {
		ok := false

		client, err := dctk.NewClient(dctk.ClientConf{
			LogLevel:         dctk.LogLevelError,
			HubUrl:           e.Url(),
			HubManualConnect: true,
			Nick:             "testdctk",
			Ip:               dockerIp,
			IsPassive:        true,
		})
		require.NoError(t, err)

		os.RemoveAll("/tmp/testshare")
		os.Mkdir("/tmp/testshare", 0755)
		os.Mkdir("/tmp/testshare/folder", 0755)
		ioutil.WriteFile("/tmp/testshare/folder/first file.txt", []byte(strings.Repeat("A", 50000)), 0644)

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
