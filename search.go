package dctoolkit

import (
	"fmt"
	"strings"
)

// SearchType contains the search type.
type SearchType int

const (
	// SearchAny searches for a file or directory by name
	SearchAny SearchType = iota
	// SearchDirectory searches for a directory by name
	SearchDirectory
	// SearchTTH searches for a file by TTH
	SearchTTH
)

// SearchResult contains a single result received after a search request.
type SearchResult struct {
	// whether the search result was received in passive or active mode
	IsActive bool
	// the peer sending the result
	Peer *Peer
	// path of a file or directory matching a search request
	Path string
	// whether the result is a directory
	IsDir bool
	// size (file only in NMDC, both files and directories in ADC)
	Size uint64
	// TTH (file only)
	TTH string
	// the available upload slots of the peer
	SlotAvail uint
}

// SearchConf allows to configure a search request.
type SearchConf struct {
	// the search type, defaults to SearchAny. See SearchType for all the available options
	Type SearchType
	// the minimum size of the searched file (if type is SearchAny or SearchTTH)
	MinSize uint64
	// the maximum size of the searched file (if type is SearchAny or SearchTTH)
	MaxSize uint64
	// part of a file name (if type is SearchAny), part of a directory name
	// (if type is SearchAny or SearchDirectory) or a TTH (if type is SearchTTH)
	Query string
}

type searchRequest struct {
	stype    SearchType
	query    string
	minSize  uint64
	maxSize  uint64
	isActive bool
}

// Search starts a file search asynchronously. See SearchConf for the available options.
func (c *Client) Search(conf SearchConf) error {
	if conf.Type == SearchTTH && TTHIsValid(conf.Query) == false {
		return fmt.Errorf("invalid TTH")
	}

	if c.protoIsAdc == true {
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
			fields[adcFieldFileTTH] = conf.Query
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

	} else {
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
			Query:   conf.Query,
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
	}
	return nil
}

func (c *Client) handleSearchRequest(req *searchRequest) ([]interface{}, error) {
	if len(req.query) < 3 {
		return nil, fmt.Errorf("query too short: %s", req.query)
	}
	if req.stype == SearchTTH && TTHIsValid(req.query) == false {
		return nil, fmt.Errorf("invalid TTH: %s", req.query)
	}

	// normalize query
	if req.stype != SearchTTH {
		req.query = strings.ToLower(req.query)
	}

	var results []interface{}
	var scanDir func(dname string, dir *shareDirectory, dirAddToResults bool)

	// search file or directory by name
	if req.stype == SearchAny || req.stype == SearchDirectory {
		scanDir = func(dname string, dir *shareDirectory, dirAddToResults bool) {
			// always add directories
			if dirAddToResults == false {
				dirAddToResults = strings.Contains(strings.ToLower(dname), req.query)
			}
			if dirAddToResults {
				results = append(results, dir)
			}

			if req.stype != SearchDirectory {
				for fname, file := range dir.files {
					fileAddToResults := dirAddToResults
					if fileAddToResults == false {
						fileAddToResults = strings.Contains(strings.ToLower(fname), req.query) &&
							(req.minSize == 0 || file.size > req.minSize) &&
							(req.maxSize == 0 || file.size < req.maxSize)
					}
					if fileAddToResults {
						results = append(results, file)
					}
				}
			}

			for sname, sdir := range dir.dirs {
				scanDir(sname, sdir, dirAddToResults)
			}
		}

		// search file by TTH
	} else {
		scanDir = func(dname string, dir *shareDirectory, dirAddToResults bool) {
			for _, file := range dir.files {
				if file.tth == req.query {
					results = append(results, file)
				}
			}
			for sname, sdir := range dir.dirs {
				scanDir(sname, sdir, false)
			}
		}
	}

	// start searching
	for alias, dir := range c.shareTree {
		scanDir(alias, dir, false)
	}

	// Implementations should send a maximum of 5 search results to passive users
	// and 10 search results to active users
	if req.isActive == true {
		if len(results) > 10 {
			results = results[:10]
		}
	} else {
		if len(results) > 5 {
			results = results[:5]
		}
	}

	dolog(LevelInfo, "[search] req: %+v | sent %d results", req, len(results))
	return results, nil
}

func (c *Client) handleSearchResult(sr *SearchResult) {
	dolog(LevelInfo, "[search] res: %+v", sr)
	if c.OnSearchResult != nil {
		c.OnSearchResult(sr)
	}
}
