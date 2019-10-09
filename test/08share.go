// +build ignore

package main

import (
	"fmt"
	"os"
	"time"

	dctk "github.com/aler9/dctoolkit"
)

func main() {
	ok := false
	dctk.SetLogLevel(dctk.LevelDebug)

	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:           os.Getenv("HUBURL"),
		HubManualConnect: true,
		Nick:             "testdctk",
		PrivateIp:        true,
		IsPassive:        true,
	})
	if err != nil {
		panic(err)
	}

	client.OnInitialized = func() {
		client.ShareAdd("etc", "/etc")
	}

	reindexed := false
	client.OnShareIndexed = func() {
		fmt.Println("indexed")

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

	if ok == false {
		panic("test failed")
	}
}
