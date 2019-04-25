package dctoolkit

import (
    "time"
)

const (
	_HUB_KEEPALIVE_PERIOD = 120 * time.Second
)

type hubKeepAliver struct {
	terminate chan struct{}
	done      chan struct{}
}

func newHubKeepAliver(h *connHub) *hubKeepAliver {
	ka := &hubKeepAliver{
		terminate: make(chan struct{}, 1),
		done:      make(chan struct{}),
	}

	go func() {
		defer func() { ka.done <- struct{}{} }()

		ticker := time.NewTicker(_HUB_KEEPALIVE_PERIOD)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// we must call Safe() since conn.Write() is not thread safe
				h.client.Safe(func() {
					if h.client.protoIsAdc == true {
						// ADC uses the TCP keepalive feature or empty packets
					} else {
						h.conn.Write(&msgNmdcKeepAlive{})
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
	ka.terminate <- struct{}{}
	<-ka.done
}
