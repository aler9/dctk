package dctk

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"github.com/aler9/go-dc/adc"
	godctiger "github.com/aler9/go-dc/tiger"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/protoadc"
	"github.com/aler9/dctk/pkg/tiger"
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
		if tiger.Hash(*msg.TTH) == dirTTH {
			sr.IsDir = true
		} else {
			sr.TTH = (*tiger.Hash)(msg.TTH)
		}
	}

	if sr.IsDir {
		sr.Path = strings.TrimSuffix(sr.Path, "/")
	}

	c.handleSearchResult(sr)
}

func (c *Client) handleAdcSearchOutgoingRequest(conf SearchConf) error {
	req := &adc.SearchRequest{
		// always add token even if we're not using it
		Token: protoadc.AdcRandomToken(),
	}

	switch conf.Type {
	case SearchAny:
		req.And = append(req.And, conf.Query)

	case SearchDirectory:
		req.Type = adc.FileTypeDir
		req.And = append(req.And, conf.Query)

	case SearchTTH:
		req.TTH = (*godctiger.Hash)(&conf.TTH)
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
	if c.conf.IsPassive {
		features = append(features, adc.FeatureSel{adc.FeaTCP4, true}) //nolint:govet
	}

	if len(features) > 0 {
		c.hubConn.conn.Write(&protoadc.AdcFSearchRequest{ //nolint:govet
			&adc.FeaturePacket{ID: c.adcSessionID, Sel: features},
			req,
		})
	} else {
		c.hubConn.conn.Write(&protoadc.AdcBSearchRequest{ //nolint:govet
			&adc.BroadcastPacket{ID: c.adcSessionID},
			req,
		})
	}
	return nil
}

func (c *Client) handleAdcSearchIncomingRequest(id adc.SID, req *adc.SearchRequest) {
	var peer *Peer
	results, err := func() ([]interface{}, error) {
		peer = c.peerBySessionID(id)
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
			isActive: !peer.IsPassive,
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
			sr.tth = tiger.Hash(*req.TTH)
		} else {
			sr.query = req.And[0]
		}

		return c.handleSearchIncomingRequest(sr)
	}()
	if err != nil {
		log.Log(c.conf.LogLevel, log.LevelDebug, "[search] error: %s", err)
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
			msg.TTH = (*godctiger.Hash)(&o.tth)
			msg.Size = int64(o.size)

		case *shareDirectory:
			// if directory, add a trailing slash
			msg.Path = o.aliasPath + "/"
			msg.TTH = (*godctiger.Hash)(&dirTTH)
			msg.Size = int64(o.size)
		}

		// add token if sent by author
		msg.Token = req.Token

		msgs = append(msgs, msg)
	}

	// send to peer
	if !peer.IsPassive {
		go func() {
			conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", peer.IP, peer.adcUDPPort))
			if err != nil {
				return
			}
			defer conn.Close()

			for _, msg := range msgs {
				amsg := &protoadc.AdcUSearchResult{ //nolint:govet
					&adc.UDPPacket{ID: peer.adcClientID},
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
			c.hubConn.conn.Write(&protoadc.AdcDSearchResult{ //nolint:govet
				&adc.DirectPacket{ID: c.adcSessionID, To: peer.adcSessionID},
				msg,
			})
		}
	}
}
