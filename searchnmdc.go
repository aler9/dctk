package dctk

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"github.com/aler9/go-dc/nmdc"
	godctiger "github.com/aler9/go-dc/tiger"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/tiger"
)

func nmdcSearchEscape(in string) string {
	return strings.Replace(in, " ", "$", -1)
}

func nmdcSearchUnescape(in string) string {
	return strings.Replace(in, "$", " ", -1)
}

func (c *Client) handleNmdcSearchResult(isActive bool, msg *nmdc.SR) {
	peer := c.peerByNick(msg.From)
	if peer == nil {
		return
	}

	sr := &SearchResult{
		IsActive:  isActive,
		Peer:      peer,
		Path:      strings.Join(msg.Path, "/"),
		SlotAvail: uint(msg.FreeSlots),
		Size:      msg.Size,
		TTH:       (*tiger.Hash)(msg.TTH),
		IsDir:     msg.IsDir,
	}
	c.handleSearchResult(sr)
}

func (c *Client) handleNmdcSearchOutgoingRequest(conf SearchConf) error {
	if conf.MaxSize != 0 && conf.MinSize != 0 {
		return fmt.Errorf("max size and min size cannot be used together in NMDC")
	}

	c.hubConn.conn.Write(&nmdc.Search{
		DataType: func() nmdc.DataType {
			switch conf.Type {
			case SearchAny:
				return nmdc.DataTypeAny
			case SearchDirectory:
				return nmdc.DataTypeFolders
			}
			return nmdc.DataTypeTTH
		}(),
		SizeRestricted: (conf.MaxSize != 0) || (conf.MinSize != 0),
		IsMaxSize:      (conf.MaxSize != 0),
		Size: func() uint64 {
			if conf.MaxSize != 0 {
				return conf.MaxSize
			}
			return conf.MinSize
		}(),
		Pattern: func() string {
			if conf.Type != SearchTTH {
				return conf.Query
			}
			return ""
		}(),
		TTH: func() *godctiger.Hash {
			if conf.Type == SearchTTH {
				ptr := godctiger.Hash(conf.TTH)
				return &ptr
			}
			return nil
		}(),
		Address: func() string {
			if !c.conf.IsPassive {
				return fmt.Sprintf("%s:%d", c.ip, c.conf.UdpPort)
			}
			return ""
		}(),
		User: func() string {
			if c.conf.IsPassive {
				return c.conf.Nick
			}
			return ""
		}(),
	})
	return nil
}

func (c *Client) handleNmdcSearchIncomingRequest(req *nmdc.Search) {
	results, err := func() ([]interface{}, error) {
		// we do not support search by type
		if _, ok := map[nmdc.DataType]struct{}{
			nmdc.DataTypeAny:     {},
			nmdc.DataTypeFolders: {},
			nmdc.DataTypeTTH:     {},
		}[req.DataType]; !ok {
			return nil, fmt.Errorf("unsupported search type: %v", req.Type)
		}

		sr := &searchIncomingRequest{
			isActive: req.Address != "",
			stype: func() SearchType {
				switch req.DataType {
				case nmdc.DataTypeAny:
					return SearchAny
				case nmdc.DataTypeFolders:
					return SearchDirectory
				}
				return SearchTTH
			}(),
			minSize: func() uint64 {
				if req.SizeRestricted && !req.IsMaxSize {
					return req.Size
				}
				return 0
			}(),
			maxSize: func() uint64 {
				if req.SizeRestricted && req.IsMaxSize {
					return req.Size
				}
				return 0
			}(),
		}

		if req.DataType == nmdc.DataTypeTTH {
			sr.tth = tiger.Hash(*req.TTH)
		} else {
			sr.query = req.Pattern
		}

		return c.handleSearchIncomingRequest(sr)
	}()
	if err != nil {
		log.Log(c.conf.LogLevel, log.LevelDebug, "[search] error: %s", err)
		return
	}

	var msgs []*nmdc.SR
	for _, res := range results {
		msgs = append(msgs, &nmdc.SR{
			Path: strings.Split(func() string {
				if f, ok := res.(*shareFile); ok {
					return f.aliasPath
				}
				return res.(*shareDirectory).aliasPath
			}(), "\\"),
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
			TTH: func() *godctiger.Hash {
				if f, ok := res.(*shareFile); ok {
					ptr := godctiger.Hash(f.tth)
					return &ptr
				}
				return nil
			}(),
			From:       c.conf.Nick,
			FreeSlots:  int(c.uploadSlotAvail),
			TotalSlots: int(c.conf.UploadMaxParallel),
			HubAddress: fmt.Sprintf("%s:%d", c.hubSolvedIp, c.hubPort),
		})
	}

	// if request was active, send to peer
	if req.Address != "" {
		go func() {
			conn, err := net.Dial("udp", req.Address)
			if err != nil {
				return
			}
			defer conn.Close()

			for _, msg := range msgs {
				var buf bytes.Buffer
				buf.WriteString("$SR ")
				msg.MarshalNMDC(nil, &buf)
				buf.WriteByte('|')
				conn.Write(buf.Bytes())
			}
		}()

		// send to hub
	} else {
		for _, msg := range msgs {
			msg.To = req.User
			c.hubConn.conn.Write(msg)
		}
	}
}
