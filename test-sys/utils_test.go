package dctoolkit_test

import (
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func getPrivateIp() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

type externalHubDef struct {
	name  string
	proto string
	port  uint
}

var externalHubDefs = []*externalHubDef{
	{"godcpp-adc", "adc", 1411},
	{"godcpp-nmdc", "nmdc", 1411},
	{"luadch", "adc", 5000},
	{"verlihub", "nmdc", 4111},
}

func foreachExternalHub(t *testing.T, cb func(t *testing.T, e *externalHub)) {
	for _, def := range externalHubDefs {
		t.Run(def.name, func(t *testing.T) {
			e := newExternalHub(def)
			defer e.close()
			cb(t, e)
		})
	}
}

type externalHub struct {
	su string
}

func newExternalHub(def *externalHubDef) *externalHub {
	exec.Command("docker", "kill", "dctk-test-sys-hub").Run()
	exec.Command("docker", "wait", "dctk-test-sys-hub").Run()
	exec.Command("docker", "rm", "dctk-test-sys-hub").Run()

	// start hub
	cmd := []string{"docker", "run", "--rm", "-d", "--name=dctk-test-sys-hub"}
	cmd = append(cmd, "dctk-test-sys-hub-"+def.name)
	exec.Command(cmd[0], cmd[1:]...).Run()

	// get hub ip
	byts, _ := exec.Command("docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
		"dctk-test-sys-hub").Output()
	ip := string(byts[:len(byts)-1])

	address := ip + ":" + strconv.FormatUint(uint64(def.port), 10)

	// wait for hub
	for {
		time.Sleep(1 * time.Second)
		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			continue
		}
		conn.Close()
		break
	}

	return &externalHub{
		su: def.proto + "://" + address,
	}
}

func (e *externalHub) Url() string {
	return e.su
}

func (e *externalHub) close() {
	exec.Command("docker", "kill", "dctk-test-sys-hub").Run()
	exec.Command("docker", "wait", "dctk-test-sys-hub").Run()
}
