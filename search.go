package dctoolkit

import (
    "fmt"
    "strings"
    "net"
    "path/filepath"
)

type SearchType int
const (
    TypeInvalid     SearchType = 0
    TypeAny         SearchType = 1
    TypeAudio       SearchType = 2
    TypeCompressed  SearchType = 3
    TypeDocument    SearchType = 4
    TypeExe         SearchType = 5
    TypePicture     SearchType = 6
    TypeVideo       SearchType = 7
    TypeFolder      SearchType = 8
    TypeTTH         SearchType = 9
)

func searchEscape(in string) string {
    return strings.Replace(in, " ", "$", -1)
}

func searchUnescape(in string) string {
    return strings.Replace(in, "$", " ", -1)
}

type SearchResult struct {
    // whether the search result was received in passive or active mode
    IsActive    bool
    // the nickname of the peer sending the result
    Nick        string
    // the path to a file matching a search request
    Path        string
    // the currently available upload slots of the peer
    SlotAvail   uint
    // the total number of the upload slots of the peer
    SlotCount   uint
    // the file TTH
    TTH         string
    // whether the result is a directory
    IsDir       bool
    // the hub ip
    HubIp       string
    // the hub port
    HubPort     uint
}

type SearchConf struct {
    // the search type, defaults to TypeAny. See SearchType for all the available options
    Type        SearchType
    // the minimum size of the file you want to search
    MinSize     uint
    // the maximum size of the file you want to search
    MaxSize     uint
    // part of a file name, a directory name (if type is TypeFolder) or a TTH (if type is TypeTTH)
    Query       string
}

// Search starts a file search asynchronously. See SearchConf for the available options.
func (c *Client) Search(conf SearchConf) error {
    if conf.Type == TypeInvalid {
        conf.Type = TypeAny
    }
    if conf.MaxSize != 0 && conf.MinSize != 0 {
        return fmt.Errorf("max size and min size cannot be used together")
    }
    if conf.Type == TypeTTH && TTHIsValid(conf.Query) == false {
        return fmt.Errorf("invalid TTH")
    }
    if c.conf.ModePassive == false && c.ip == "" {
        return fmt.Errorf("we do not know our ip")
    }

    c.hubConn.conn.SendQueued(&msgNmdcSearchRequest{
        Type: conf.Type,
        MaxSize: conf.MaxSize,
        MinSize: conf.MinSize,
        Query: conf.Query,
        Ip: func() string {
            if c.conf.ModePassive == false {
                return c.ip
            }
            return ""
        }(),
        UdpPort: func() uint {
            if c.conf.ModePassive == false {
                return c.conf.UdpPort
            }
            return 0
        }(),
        Nick: func() string {
            if c.conf.ModePassive == true {
                return c.conf.Nick
            }
            return ""
        }(),
    })
    return nil
}

func (c *Client) onSearchRequest(req *msgNmdcSearchRequest) {
    if req.Type == TypeInvalid {
        return
    }
    if len(req.Query) < 3 {
        return
    }
    if req.Type == TypeTTH &&
        (!strings.HasPrefix(req.Query, "TTH:") || !TTHIsValid(req.Query[4:])) {
        return
    }

    // normalize query
    if req.Type != TypeTTH {
        req.Query = strings.ToLower(req.Query)
    }

    var replies []*msgNmdcSearchResult
    var scanDir func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool)

    // search by file name
    if req.Type != TypeTTH {
        scanDir = func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool) {
            if dirAddToResults == false {
                dirAddToResults = (req.Type == TypeAny || req.Type == TypeFolder) &&
                    strings.Contains(strings.ToLower(dname), req.Query)
            }
            if dirAddToResults {
                replies = append(replies, &msgNmdcSearchResult{
                    Path: filepath.Join(dpath, dname),
                    IsDir: true,
                })
            }
            for fname,file := range dir.files {
                fileAddToResults := dirAddToResults
                if fileAddToResults == false {
                    fileAddToResults = req.Type != TypeFolder &&
                        strings.Contains(strings.ToLower(fname), req.Query)
                }
                if fileAddToResults {
                    replies = append(replies, &msgNmdcSearchResult{
                        Path: filepath.Join(dpath, dname, fname),
                        IsDir: false,
                        TTH: file.tth,
                    })
                }
            }
            for sname,sdir := range dir.dirs {
                scanDir(filepath.Join(dpath, dname), sname, sdir, dirAddToResults)
            }
        }

    // search by TTH
    } else {
        scanDir = func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool) {
            for fname,file := range dir.files {
                if file.tth == req.Query[4:] {
                    replies = append(replies, &msgNmdcSearchResult{
                        Path: filepath.Join(dpath, dname, fname),
                        IsDir: false,
                        TTH: file.tth,
                    })
                }
            }
            for sname,sdir := range dir.dirs {
                scanDir(filepath.Join(dpath, dname), sname, sdir, false)
            }
        }
    }

    // start searching
    for alias,dir := range c.shareTree {
        scanDir("", alias, dir.shareDirectory, false)
    }

    // Implementations should send a maximum of 5 search results to passive users
    // and 10 search results to active users
    if req.IsActive == true {
        if len(replies) > 10 {
            replies = replies[:10]
        }
    } else {
        if len(replies) > 5 {
            replies = replies[:5]
        }
    }

    // fill additional data
    for _,msg := range replies {
        msg.Nick = c.conf.Nick
        msg.SlotAvail = c.uploadSlotAvail
        msg.SlotCount = c.conf.UploadMaxParallel
        msg.HubIp = c.hubSolvedIp
        msg.HubPort = c.hubPort
    }

    if req.IsActive == true {
        // send to peer
        go func() {
            conn,err := net.Dial("udp", fmt.Sprintf("%s:%d", req.Ip, req.UdpPort))
            if err != nil {
                return
            }
            defer conn.Close()

            for _,msg := range replies {
                conn.Write(msg.Encode())
            }
        }()

    } else {
        // send to hub
        for _,msg := range replies {
            msg.TargetNick = req.Nick
            c.hubConn.conn.SendQueued(msg)
        }
    }

    dolog(LevelInfo, "[search req] %+v | sent %d results", req, len(replies))
}
