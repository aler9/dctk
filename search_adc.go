package dctoolkit

import (
	"fmt"
	"net"
	"strings"
)

const (
	adcSearchFile      = "1"
	adcSearchDirectory = "2"
)

func (c *Client) handleAdcSearchResult(isActive bool, peer *Peer, msg *msgAdcKeySearchResult) {
	sr := &SearchResult{
		IsActive: isActive,
		Peer:     peer,
	}

	for key, val := range msg.Fields {
		switch key {
		case adcFieldFilePath:
			sr.Path = val
		case adcFieldSize:
			sr.Size = atoui64(val)
		case adcFieldFileTTH:
			if val == dirTTH {
				sr.IsDir = true
			} else {
				err := sr.TTH.FromBase32(val)
				if err != nil {
					return
				}
			}
		case adcFieldUploadSlotCount:
			sr.SlotAvail = atoui(val)
		}
	}
	if sr.IsDir == true {
		sr.Path = strings.TrimSuffix(sr.Path, "/")
	}

	c.handleSearchResult(sr)
}

func (c *Client) handleAdcSearchOutgoingRequest(conf SearchConf) error {
	fields := make(map[string]string)

	// always add token even if we're not using it
	fields[adcFieldToken] = adcRandomToken()

	switch conf.Type {
	case SearchAny:
		fields[adcFieldQueryAnd] = conf.Query

	case SearchDirectory:
		fields[adcFieldIsFileOrDir] = adcSearchDirectory
		fields[adcFieldQueryAnd] = conf.Query

	case SearchTTH:
		fields[adcFieldFileTTH] = conf.TTH.String()
	}

	// MaxSize and MinSize are used only for files. They can be used for
	// directories too in ADC, but we want to minimize differences with NMDC.
	if conf.Type == SearchAny || conf.Type == SearchTTH {
		if conf.MaxSize != 0 {
			fields[adcFieldMaxSize] = numtoa(conf.MaxSize)
		}
		if conf.MinSize != 0 {
			fields[adcFieldMinSize] = numtoa(conf.MinSize)
		}
	}

	requiredFeatures := make(map[string]struct{})

	// if we're passive, require that the recipient is active
	if c.conf.IsPassive == true {
		requiredFeatures["TCP4"] = struct{}{}
	}

	if len(requiredFeatures) > 0 {
		c.connHub.conn.Write(&msgAdcFSearchRequest{
			msgAdcTypeF{SessionId: c.sessionId, RequiredFeatures: requiredFeatures},
			msgAdcKeySearchRequest{fields},
		})
	} else {
		c.connHub.conn.Write(&msgAdcBSearchRequest{
			msgAdcTypeB{c.sessionId},
			msgAdcKeySearchRequest{fields},
		})
	}
	return nil
}

func (c *Client) handleAdcSearchIncomingRequest(authorSessionId string, req *msgAdcKeySearchRequest) {
	var peer *Peer
	results, err := func() ([]interface{}, error) {
		peer = c.peerBySessionId(authorSessionId)
		if peer == nil {
			return nil, fmt.Errorf("search author not found")
		}

		if _, ok := req.Fields[adcFieldFileGroup]; ok {
			return nil, fmt.Errorf("search by type is not supported")
		}
		if _, ok := req.Fields[adcFieldFileExcludeExtens]; ok {
			return nil, fmt.Errorf("search by type is not supported")
		}
		if _, ok := req.Fields[adcFieldFileQueryOr]; ok {
			return nil, fmt.Errorf("search by query OR is not supported")
		}
		if _, ok := req.Fields[adcFieldFileExactSize]; ok {
			return nil, fmt.Errorf("search by exact size is not supported")
		}
		if _, ok := req.Fields[adcFieldFileExtension]; ok {
			return nil, fmt.Errorf("search by extension is not supported")
		}
		if _, ok := req.Fields[adcFieldIsFileOrDir]; ok {
			if req.Fields[adcFieldIsFileOrDir] != adcSearchDirectory {
				return nil, fmt.Errorf("search file only is not supported")
			}
		}
		if _, ok := req.Fields[adcFieldQueryAnd]; !ok {
			if _, ok := req.Fields[adcFieldFileTTH]; !ok {
				return nil, fmt.Errorf("AN or TR is required")
			}
		}

		return c.handleSearchIncomingRequest(&searchRequest{
			stype: func() SearchType {
				if _, ok := req.Fields[adcFieldFileTTH]; ok {
					return SearchTTH
				}
				if _, ok := req.Fields[adcFieldIsFileOrDir]; ok {
					if req.Fields[adcFieldIsFileOrDir] == adcSearchDirectory {
						return SearchDirectory
					}
				}
				return SearchAny
			}(),
			query: func() string {
				if _, ok := req.Fields[adcFieldFileTTH]; ok {
					return req.Fields[adcFieldFileTTH]
				}
				return req.Fields[adcFieldQueryAnd]
			}(),
			minSize: func() uint64 {
				if val, ok := req.Fields[adcFieldMinSize]; ok {
					return atoui64(val)
				}
				return 0
			}(),
			maxSize: func() uint64 {
				if val, ok := req.Fields[adcFieldMaxSize]; ok {
					return atoui64(val)
				}
				return 0
			}(),
			isActive: (peer.IsPassive == false),
		})
	}()
	if err != nil {
		dolog(LevelDebug, "[search] error: %s", err)
		return
	}

	var msgs []*msgAdcKeySearchResult
	for _, res := range results {
		fields := map[string]string{
			adcFieldUploadSlotCount: numtoa(c.conf.UploadMaxParallel),
		}

		switch o := res.(type) {
		case *shareFile:
			fields[adcFieldFilePath] = o.aliasPath
			fields[adcFieldFileTTH] = o.tth.String()
			fields[adcFieldSize] = numtoa(o.size)

		case *shareDirectory:
			// if directory, add a trailing slash
			fields[adcFieldFilePath] = o.aliasPath + "/"
			fields[adcFieldFileTTH] = dirTTH
			fields[adcFieldSize] = numtoa(o.size)
		}

		// add token if sent by author
		if val, ok := req.Fields[adcFieldToken]; ok {
			fields[adcFieldToken] = val
		}

		msgs = append(msgs, &msgAdcKeySearchResult{Fields: fields})
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
				encmsg := &msgAdcUSearchResult{
					msgAdcTypeU{peer.adcClientId},
					*msg,
				}
				conn.Write([]byte(encmsg.AdcTypeEncode(encmsg.AdcKeyEncode())))
			}
		}()

		// send to hub
	} else {
		for _, msg := range msgs {
			c.connHub.conn.Write(&msgAdcDSearchResult{
				msgAdcTypeD{c.sessionId, peer.adcSessionId},
				*msg,
			})
		}
	}
}
