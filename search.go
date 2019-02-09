package dctoolkit

import (
    "fmt"
    "strings"
    "regexp"
    "net"
    "path/filepath"
)

var reCmdSearchReqActive = regexp.MustCompile("^("+reStrIp+"):("+reStrPort+") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")
var reCmdSearchReqPassive = regexp.MustCompile("^Hub:("+reStrNick+") (F|T)\\?(F|T)\\?([0-9]+)\\?([0-9])\\?(.+)$")
var reCmdSearchRes = regexp.MustCompile("^("+reStrNick+") (.+?) ([0-9]+)/([0-9]+)\x05TTH:("+reStrTTH+") \\(("+reStrIp+"):("+reStrPort+")\\)$")

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

    req := &searchRequest{
        Type: conf.Type,
        MaxSize: conf.MaxSize,
        MinSize: conf.MinSize,
        Query: conf.Query,
        IsActive: c.conf.ModePassive == false,
    }
    if req.IsActive {
        req.Ip = c.ip
        req.UdpPort = c.conf.UdpPort
    } else {
        req.Nick = c.conf.Nick
    }

    c.hubConn.conn.Send <- req.export()
    return nil
}

type searchRequest struct {
    Type        SearchType
    MinSize     uint
    MaxSize     uint
    Query       string
    IsActive    bool
    Ip          string  // active only
    UdpPort     uint    // active only
    Nick        string  // passive only
}

func newSearchRequest(args string) (*searchRequest,error) {
    var req *searchRequest
    if parsed := reCmdSearchReqActive.FindStringSubmatch(args); parsed != nil {
        req = &searchRequest{
            IsActive: true,
            Ip: parsed[1],
            UdpPort: atoui(parsed[2]),
            MaxSize: func() uint {
                if parsed[3] == "T" && parsed[4] == "T" {
                    return atoui(parsed[5])
                }
                return 0
            }(),
            MinSize: func() uint {
                if parsed[3] == "T" && parsed[4] == "F" {
                    return atoui(parsed[5])
                }
                return 0
            }(),
            Type: SearchType(atoi(parsed[6])),
            Query: searchUnescape(parsed[7]),
        }
    } else if parsed := reCmdSearchReqPassive.FindStringSubmatch(args); parsed != nil {
        req = &searchRequest{
            IsActive: false,
            Nick: parsed[1],
            MaxSize: func() uint {
                if parsed[2] == "T" && parsed[3] == "T" {
                    return atoui(parsed[4])
                }
                return 0
            }(),
            MinSize: func() uint {
                if parsed[2] == "T" && parsed[3] == "F" {
                    return atoui(parsed[4])
                }
                return 0
            }(),
            Type: SearchType(atoi(parsed[5])),
            Query: searchUnescape(parsed[6]),
        }
    } else {
        return nil, fmt.Errorf("invalid args")
    }

    if len(req.Query) < 3 {
        return nil, fmt.Errorf("query too short")
    }
    if req.Type == TypeTTH &&
        (!strings.HasPrefix(req.Query, "TTH:") || !TTHIsValid(req.Query[4:])) {
        return nil, fmt.Errorf("invalid TTH")
    }

    return req, nil
}

func (req *searchRequest) export() msgBase {
    // https://web.archive.org/web/20150323121346/http://wiki.gusari.org/index.php?title=$Search
    // http://nmdc.sourceforge.net/Versions/NMDC-1.3.html#_search
    // <sizeRestricted>?<isMaxSize>?<size>?<fileType>?<searchPattern>
    string := fmt.Sprintf("%s %s?%s?%d?%d?%s",
        func() string {
            if req.IsActive {
                return fmt.Sprintf("%s:%d", req.Ip, req.UdpPort)
            }
            // Hub: is prefixed by hub
            return fmt.Sprintf("Hub:%s", req.Nick)
        }(),
        func() string {
            if req.MinSize != 0 || req.MaxSize != 0 {
                return "T"
            }
            return "F"
        }(),
        func() string {
            if req.MaxSize != 0 {
                return "T"
            }
            return "F"
        }(),
        func() uint {
            if req.MaxSize != 0 {
                return req.MaxSize
            }
            return req.MinSize
        }(),
        req.Type,
        func() string {
            if req.Type == TypeTTH {
                return "TTH:" + req.Query
            }
            return searchEscape(req.Query)
        }())

    return msgCommand{ "Search", string }
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
    // field for SENDING (not receiving) search results in passive mode, stripped by the hub
    TargetNick  string
}

func newSearchResult(isActive bool, args string) (*SearchResult,error) {
    parsed := reCmdSearchRes.FindStringSubmatch(args)
    if parsed == nil {
        return nil, fmt.Errorf("unable to parse")
    }

    res := &SearchResult{
        IsActive: isActive,
        Nick: parsed[1],
        Path: parsed[2],
        SlotAvail: atoui(parsed[3]),
        SlotCount: atoui(parsed[4]),
        TTH: func() string {
            if parsed[5] != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" {
                return parsed[5]
            }
            return ""
        }(),
        IsDir: func() bool {
            return parsed[5] == "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
        }(),
        HubIp: parsed[6],
        HubPort: atoui(parsed[7]),
    }
    return res, nil
}

func (res *SearchResult) export() msgBase {
    string := fmt.Sprintf("%s %s %d/%d\x05TTH:%s (%s:%d)",
        res.Nick,
        res.Path,
        res.SlotAvail,
        res.SlotCount,
        func() string {
            if res.IsDir {
                return "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
            }
            return res.TTH
        }(),
        res.HubIp,
        res.HubPort)

    if res.IsActive == false {
        string += "\x05" + res.TargetNick
    }

    return msgCommand{ Key: "SR", Args: string }
}

func searchEscape(in string) string {
    return strings.Replace(in, " ", "$", -1)
}

func searchUnescape(in string) string {
    return strings.Replace(in, "$", " ", -1)
}

func (c *Client) onSearchRequest(req *searchRequest) {
    // normalize query
    if req.Type != TypeTTH {
        req.Query = strings.ToLower(req.Query)
    }

    // find results
    var results []*SearchResult
    var scanDir func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool)
    if req.Type != TypeTTH {
        scanDir = func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool) {
            if dirAddToResults == false {
                dirAddToResults = (req.Type == TypeAny || req.Type == TypeFolder) &&
                    strings.Contains(strings.ToLower(dname), req.Query)
            }
            if dirAddToResults {
                results = append(results, &SearchResult{
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
                    results = append(results, &SearchResult{
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
    } else {
        scanDir = func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool) {
            for fname,file := range dir.files {
                if file.tth == req.Query[4:] {
                    results = append(results, &SearchResult{
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
    for alias,dir := range c.shareTree {
        scanDir("", alias, dir.shareDirectory, false)
    }

    // Implementations should send a maximum of 5 search results to passive users
    // and 10 search results to active users
    if req.IsActive {
        if len(results) > 10 {
            results = results[:10]
        }
    } else {
        if len(results) > 5 {
            results = results[:5]
        }
    }

    // fill additional data
    for _,res := range results {
        res.IsActive = req.IsActive
        res.Nick = c.conf.Nick
        res.SlotAvail = c.uploadSlotAvail
        res.SlotCount = c.conf.UploadMaxParallel
        res.HubIp = c.hubSolvedIp
        res.HubPort = c.hubPort
    }

    if req.IsActive {
        go func() {
            // connect to peer
            conn,err := net.Dial("udp", fmt.Sprintf("%s:%d", req.Ip, req.UdpPort))
            if err != nil {
                return
            }
            defer conn.Close()

            // send to peer
            for _,res := range results {
                conn.Write(res.export().Bytes())
            }
        }()
    } else {
        // send to hub
        for _,res := range results {
            res.TargetNick = req.Nick
            c.hubConn.conn.Send <- res.export()
        }
    }

    dolog(LevelInfo, "[search req] %+v | sent %d results", req, len(results))
}
