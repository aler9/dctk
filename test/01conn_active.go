// +build ignore

package main

import (
	"os"
	"time"

	dctk "github.com/gswly/dctoolkit"
)

func main() {
	ok := false
	dctk.SetLogLevel(dctk.LevelDebug)

	client, err := dctk.NewClient(dctk.ClientConf{
		HubUrl:     os.Getenv("HUBURL"),
		Nick:       "testdctk",
		PrivateIp:  true,
		TcpPort:    3006,
		UdpPort:    3006,
		TcpTlsPort: 3007,
	})
	if err != nil {
		panic(err)
	}

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

	if ok == false {
		panic("test failed")
	}
}
