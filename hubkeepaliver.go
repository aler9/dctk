package dctk

import (
	"time"

	"github.com/aler9/dctk/pkg/protoadc"
	"github.com/aler9/dctk/pkg/protonmdc"
)

const (
	hubKeepalivePeriod = 120 * time.Second
)

type hubKeepAliver struct {
	terminate chan struct{}
	done      chan struct{}
}

func newHubKeepAliver(h *hubConn) *hubKeepAliver {
	ka := &hubKeepAliver{
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	go func() {
		defer close(ka.done)

		ticker := time.NewTicker(hubKeepalivePeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// we must call Safe() since conn.Write() is not thread safe
				h.client.Safe(func() {
					if h.client.protoIsAdc() {
						// ADC uses the TCP keepalive feature or empty packets
						h.conn.Write(&protoadc.AdcKeepAlive{})
					} else {
						h.conn.Write(&protonmdc.NmdcKeepAlive{})
					}
				})
			case <-ka.terminate:
				return
			}
		}
	}()
	return ka
}

func (ka *hubKeepAliver) Close() {
	close(ka.terminate)
	<-ka.done
}
