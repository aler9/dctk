package dctoolkit

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/tiger"

	"github.com/aler9/dctoolkit/log"
	"github.com/aler9/dctoolkit/proto"
)

func (c *Client) handleAdcSearchResult(isActive bool, peer *Peer, msg *adc.SearchResult) {
	sr := &SearchResult{
		IsActive: isActive,
		Peer:     peer,
	}

	sr.Path = msg.Path
	sr.Size = uint64(msg.Size)
	sr.SlotAvail = uint(msg.Slots)
	if msg.TTH != nil {
		if *msg.TTH == tiger.Hash(dirTTH) {
			sr.IsDir = true
		} else {
			sr.TTH = (*TigerHash)(msg.TTH)
		}
	}

	if sr.IsDir == true {
		sr.Path = strings.TrimSuffix(sr.Path, "/")
	}

	c.handleSearchResult(sr)
}

func (c *Client) handleAdcSearchOutgoingRequest(conf SearchConf) error {
	req := &adc.SearchRequest{
		// always add token even if we're not using it
		Token: proto.AdcRandomToken(),
	}

	switch conf.Type {
	case SearchAny:
		req.And = append(req.And, conf.Query)

	case SearchDirectory:
		req.Type = adc.FileTypeDir
		req.And = append(req.And, conf.Query)

	case SearchTTH:
		req.TTH = (*tiger.Hash)(&conf.TTH)
	}

	// MaxSize and MinSize are used only for files. They can be used for
	// directories too in ADC, but we want to minimize differences with NMDC.
	if conf.Type == SearchAny || conf.Type == SearchTTH {
		if conf.MaxSize != 0 {
			req.Le = int64(conf.MaxSize)
		}
		if conf.MinSize != 0 {
			req.Ge = int64(conf.MinSize)
		}
	}

	var features []adc.FeatureSel

	// if we're passive, require that the receiver is active
	if c.conf.IsPassive == true {
		features = append(features, adc.FeatureSel{adc.FeaTCP4, true})
	}

	if len(features) > 0 {
		c.hubConn.conn.Write(&proto.AdcFSearchRequest{
			&adc.FeaturePacket{ID: c.adcSessionId, Sel: features},
			req,
		})
	} else {
		c.hubConn.conn.Write(&proto.AdcBSearchRequest{
			&adc.BroadcastPacket{ID: c.adcSessionId},
			req,
		})
	}
	return nil
}

func (c *Client) handleAdcSearchIncomingRequest(ID adc.SID, req *adc.SearchRequest) {
	var peer *Peer
	results, err := func() ([]interface{}, error) {
		peer = c.peerBySessionId(ID)
		if peer == nil {
			return nil, fmt.Errorf("search author not found")
		}

		if req.Group != adc.ExtNone {
			return nil, fmt.Errorf("search by type is not supported")
		}
		if len(req.Ext) > 0 {
			return nil, fmt.Errorf("search by extension is not supported")
		}
		if len(req.Not) > 0 {
			return nil, fmt.Errorf("search by OR is not supported")
		}
		if req.Eq != 0 {
			return nil, fmt.Errorf("search by exact size is not supported")
		}
		if req.Type == adc.FileTypeFile {
			return nil, fmt.Errorf("file-only search is not supported")
		}

		if len(req.And) == 0 && req.TTH == nil {
			return nil, fmt.Errorf("AN or TR are required")
		}

		sr := &searchIncomingRequest{
			isActive: (peer.IsPassive == false),
			stype: func() SearchType {
				if req.TTH != nil {
					return SearchTTH
				}
				if req.Type == adc.FileTypeDir {
					return SearchDirectory
				}
				return SearchAny
			}(),
			minSize: uint64(req.Ge),
			maxSize: uint64(req.Le),
		}

		if req.TTH != nil {
			sr.tth = TigerHash(*req.TTH)
		} else {
			sr.query = req.And[0]
		}

		return c.handleSearchIncomingRequest(sr)
	}()
	if err != nil {
		log.Log(c.conf.LogLevel, LogLevelDebug, "[search] error: %s", err)
		return
	}

	var msgs []*adc.SearchResult
	for _, res := range results {
		msg := &adc.SearchResult{
			Slots: int(c.conf.UploadMaxParallel),
		}

		switch o := res.(type) {
		case *shareFile:
			msg.Path = o.aliasPath
			msg.TTH = (*tiger.Hash)(&o.tth)
			msg.Size = int64(o.size)

		case *shareDirectory:
			// if directory, add a trailing slash
			msg.Path = o.aliasPath + "/"
			msg.TTH = (*tiger.Hash)(&dirTTH)
			msg.Size = int64(o.size)
		}

		// add token if sent by author
		msg.Token = req.Token

		msgs = append(msgs, msg)
	}

	// send to peer
	if peer.IsPassive == false {
		go func() {
			conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", peer.Ip, peer.adcUdpPort))
			if err != nil {
				return
			}
			defer conn.Close()

			for _, msg := range msgs {
				amsg := &proto.AdcUSearchResult{
					&adc.UDPPacket{ID: peer.adcClientId},
					msg,
				}

				amsg.Pkt.SetMessage(amsg.Msg)

				var buf bytes.Buffer
				if err := amsg.Pkt.MarshalPacketADC(&buf); err != nil {
					panic(err)
				}

				conn.Write(buf.Bytes())
			}
		}()

		// send to hub
	} else {
		for _, msg := range msgs {
			c.hubConn.conn.Write(&proto.AdcDSearchResult{
				&adc.DirectPacket{ID: c.adcSessionId, To: peer.adcSessionId},
				msg,
			})
		}
	}
}
