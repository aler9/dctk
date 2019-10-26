package dctoolkit_test

import (
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"
)

var externalHubNames = func() []string {
	var ret []string
	files, _ := ioutil.ReadDir(".")
	for _, f := range files {
		if f.IsDir() {
			ret = append(ret, f.Name())
		}
	}
	return ret
}()

func foreachExternalHub(t *testing.T, cb func(t *testing.T, e *externalHub)) {
	for _, name := range externalHubNames {
		t.Run(name, func(t *testing.T) {
			e := newExternalHub(name)
			defer e.close()
			cb(t, e)
		})
	}
}

type externalHub struct {
	su string
}

func newExternalHub(name string) *externalHub {
	exec.Command("docker", "kill", "dctk-test-sys-hub").Run()
	exec.Command("docker", "wait", "dctk-test-sys-hub").Run()
	exec.Command("docker", "rm", "dctk-test-sys-hub").Run()

	// start hub
	cmd := []string{"docker", "run", "--rm", "-d", "--name=dctk-test-sys-hub"}
	if os.Getenv("IN_DOCKER") == "1" {
		cmd = append(cmd, "--network=container:dctk-test-sys")
	} else {
		cmd = append(cmd, "--network=host")
	}
	cmd = append(cmd, "dctk-test-sys-hub-"+name)
	exec.Command(cmd[0], cmd[1:]...).Run()

	// get hub url
	byts, _ := ioutil.ReadFile(name + "/URL")
	su := string(byts[:len(byts)-1])

	// wait for hub
	u, _ := url.Parse(su)
	for {
		conn, err := net.DialTimeout("tcp", u.Host, 1*time.Second)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		conn.Close()
		break
	}

	return &externalHub{
		su: su,
	}
}

func (e *externalHub) Url() string {
	return e.su
}

func (e *externalHub) close() {
	exec.Command("docker", "kill", "dctk-test-sys-hub").Run()
	exec.Command("docker", "wait", "dctk-test-sys-hub").Run()
}
