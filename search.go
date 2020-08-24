package dctoolkit

import (
	"fmt"
	"strings"

	"github.com/aler9/dctoolkit/log"
	"github.com/aler9/dctoolkit/tiger"
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
	TTH *tiger.Hash
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
	// (if type is SearchAny or SearchDirectory)
	Query string
	// file TTH (if type is SearchTTH)
	TTH tiger.Hash
}

type searchIncomingRequest struct {
	isActive bool
	stype    SearchType
	minSize  uint64
	maxSize  uint64
	query    string     // if type is SearchAny or SearchDirectory
	tth      tiger.Hash // if type is SearchTTH
}

// Search starts a file search asynchronously. See SearchConf for the available options.
func (c *Client) Search(conf SearchConf) error {
	if c.protoIsAdc() {
		return c.handleAdcSearchOutgoingRequest(conf)
	}
	return c.handleNmdcSearchOutgoingRequest(conf)
}

func (c *Client) handleSearchIncomingRequest(req *searchIncomingRequest) ([]interface{}, error) {
	var results []interface{}
	var scanDir func(dname string, dir *shareDirectory, dirAddToResults bool)

	// search file or directory by name
	if req.stype == SearchAny || req.stype == SearchDirectory {
		if len(req.query) < 3 {
			return nil, fmt.Errorf("query too short: %s", req.query)
		}

		// normalize query
		req.query = strings.ToLower(req.query)

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
				if file.tth == req.tth {
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

	log.Log(c.conf.LogLevel, LogLevelInfo, "[search] req: %+v | sent %d results", req, len(results))
	return results, nil
}

func (c *Client) handleSearchResult(sr *SearchResult) {
	log.Log(c.conf.LogLevel, LogLevelInfo, "[search] res: %+v", sr)
	if c.OnSearchResult != nil {
		c.OnSearchResult(sr)
	}
}
