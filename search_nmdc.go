package dctoolkit

import (
	"fmt"
	"github.com/direct-connect/go-dc/tiger"
	"net"
	"strings"
)

type nmdcSearchType int

const (
	nmdcSearchTypeInvalid    nmdcSearchType = 0
	nmdcSearchTypeAny        nmdcSearchType = 1
	nmdcSearchTypeAudio      nmdcSearchType = 2
	nmdcSearchTypeCompressed nmdcSearchType = 3
	nmdcSearchTypeDocument   nmdcSearchType = 4
	nmdcSearchTypeExe        nmdcSearchType = 5
	nmdcSearchTypePicture    nmdcSearchType = 6
	nmdcSearchTypeVideo      nmdcSearchType = 7
	nmdcSearchTypeDirectory  nmdcSearchType = 8
	nmdcSearchTypeTTH        nmdcSearchType = 9
)

func nmdcSearchEscape(in string) string {
	return strings.Replace(in, " ", "$", -1)
}

func nmdcSearchUnescape(in string) string {
	return strings.Replace(in, "$", " ", -1)
}

func (c *Client) handleNmdcSearchResult(isActive bool, peer *Peer, msg *msgNmdcSearchResult) {
	sr := &SearchResult{
		IsActive:  isActive,
		Peer:      peer,
		Path:      msg.Path,
		SlotAvail: msg.SlotAvail,
		Size:      msg.Size,
		TTH:       msg.TTH,
		IsDir:     msg.IsDir,
	}
	c.handleSearchResult(sr)
}

func (c *Client) handleNmdcSearchOutgoingRequest(conf SearchConf) error {
	if conf.MaxSize != 0 && conf.MinSize != 0 {
		return fmt.Errorf("max size and min size cannot be used together in NMDC")
	}

	c.connHub.conn.Write(&msgNmdcSearchRequest{
		Type: func() nmdcSearchType {
			switch conf.Type {
			case SearchAny:
				return nmdcSearchTypeAny
			case SearchDirectory:
				return nmdcSearchTypeDirectory
			}
			return nmdcSearchTypeTTH
		}(),
		MaxSize: conf.MaxSize,
		MinSize: conf.MinSize,
		Query: func() string {
			if conf.Type == SearchTTH {
				return conf.TTH.String()
			}
			return conf.Query
		}(),
		Ip: func() string {
			if c.conf.IsPassive == false {
				return c.ip
			}
			return ""
		}(),
		UdpPort: func() uint {
			if c.conf.IsPassive == false {
				return c.conf.UdpPort
			}
			return 0
		}(),
		Nick: func() string {
			if c.conf.IsPassive == true {
				return c.conf.Nick
			}
			return ""
		}(),
	})
	return nil
}

func (c *Client) handleNmdcSearchIncomingRequest(req *msgNmdcSearchRequest) {
	results, err := func() ([]interface{}, error) {
		// we do not support search by type
		if _, ok := map[nmdcSearchType]struct{}{
			nmdcSearchTypeAny:       {},
			nmdcSearchTypeDirectory: {},
			nmdcSearchTypeTTH:       {},
		}[req.Type]; !ok {
			return nil, fmt.Errorf("unsupported search type: %v", req.Type)
		}
		if req.Type == nmdcSearchTypeTTH && strings.HasPrefix(req.Query, "TTH:") == false {
			return nil, fmt.Errorf("invalid TTH: %v", req.Query)
		}

		return c.handleSearchIncomingRequest(&searchRequest{
			stype: func() SearchType {
				switch req.Type {
				case nmdcSearchTypeAny:
					return SearchAny
				case nmdcSearchTypeDirectory:
					return SearchDirectory
				}
				return SearchTTH
			}(),
			query: func() string {
				if req.Type == nmdcSearchTypeTTH {
					return req.Query[4:]
				}
				return req.Query
			}(),
			minSize:  req.MinSize,
			maxSize:  req.MaxSize,
			isActive: req.IsActive,
		})
	}()
	if err != nil {
		dolog(LevelDebug, "[search] error: %s", err)
		return
	}

	var msgs []*msgNmdcSearchResult
	for _, res := range results {
		msgs = append(msgs, &msgNmdcSearchResult{
			Path: func() string {
				if f, ok := res.(*shareFile); ok {
					return f.aliasPath
				}
				return res.(*shareDirectory).aliasPath
			}(),
			IsDir: func() bool {
				_, ok := res.(*shareDirectory)
				return ok
			}(),
			Size: func() uint64 {
				if f, ok := res.(*shareFile); ok {
					return f.size
				}
				return 0
			}(),
			TTH: func() tiger.Hash {
				if f, ok := res.(*shareFile); ok {
					return f.tth
				}
				return tiger.Hash{}
			}(),
			Nick:      c.conf.Nick,
			SlotAvail: c.uploadSlotAvail,
			SlotCount: c.conf.UploadMaxParallel,
			HubIp:     c.hubSolvedIp,
			HubPort:   c.hubPort,
		})
	}

	// send to peer
	if req.IsActive == true {
		go func() {
			conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", req.Ip, req.UdpPort))
			if err != nil {
				return
			}
			defer conn.Close()

			for _, msg := range msgs {
				conn.Write([]byte(msg.NmdcEncode()))
			}
		}()

		// send to hub
	} else {
		for _, msg := range msgs {
			msg.TargetNick = req.Nick
			c.connHub.conn.Write(msg)
		}
	}
}
