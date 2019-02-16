package dctoolkit

import (
    "fmt"
    "net"
    "strings"
    "math/rand"
)

type SearchType int
const (
    // search for a file or directory by name
    SearchAny      SearchType = iota
    // search for a directory by name
    SearchDirectory
    // search for a file by TTH
    SearchTTH
)

const (
    adcSearchFile       = "1"
    adcSearchDirectory  = "2"
)

type nmdcSearchType int
const (
    nmdcSearchTypeInvalid     nmdcSearchType = 0
    nmdcSearchTypeAny         nmdcSearchType = 1
    nmdcSearchTypeAudio       nmdcSearchType = 2
    nmdcSearchTypeCompressed  nmdcSearchType = 3
    nmdcSearchTypeDocument    nmdcSearchType = 4
    nmdcSearchTypeExe         nmdcSearchType = 5
    nmdcSearchTypePicture     nmdcSearchType = 6
    nmdcSearchTypeVideo       nmdcSearchType = 7
    nmdcSearchTypeDirectory   nmdcSearchType = 8
    nmdcSearchTypeTTH         nmdcSearchType = 9
)

func adcMsgToSearchResult(isActive bool, peer *Peer, msg *msgAdcKeySearchResult) *SearchResult {
    sr := &SearchResult{
        IsActive: isActive,
        Peer: peer,
    }
    for key,val := range msg.Fields {
        switch key {
        case adcFieldFilePath: sr.Path = val
        case adcFieldFileSize: sr.Size = atoui64(val)
        case adcFieldFileTTH:
            if val == dirTTH {
                sr.IsDir = true
            } else {
                sr.TTH = val
            }
        case adcFieldUploadSlotCount: sr.SlotAvail = atoui(val)
        }
    }
    if sr.IsDir == true {
        sr.Path = strings.TrimSuffix(sr.Path, "/")
    }
    return sr
}

func adcRandomSearchToken() string {
    const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    buf := make([]byte, 10)
    for i,_ := range buf {
        buf[i] = chars[rand.Intn(len(chars))]
    }
    return string(buf)
}

func nmdcMsgToSearchResult(isActive bool, peer *Peer, msg *msgNmdcSearchResult) *SearchResult {
    return &SearchResult{
        IsActive: isActive,
        Peer: peer,
        Path: msg.Path,
        SlotAvail: msg.SlotAvail,
        Size: msg.Size,
        TTH: msg.TTH,
        IsDir: msg.IsDir,
    }
}

func nmdcSearchEscape(in string) string {
    return strings.Replace(in, " ", "$", -1)
}

func nmdcSearchUnescape(in string) string {
    return strings.Replace(in, "$", " ", -1)
}

type SearchResult struct {
    // whether the search result was received in passive or active mode
    IsActive    bool
    // the peer sending the result
    Peer        *Peer
    // path of a file or directory matching a search request
    Path        string
    // whether the result is a directory
    IsDir       bool
    // size (file only)
    Size        uint64
    // TTH (file only)
    TTH         string
    // the available upload slots of the peer
    SlotAvail   uint
}

type SearchConf struct {
    // the search type, defaults to SearchAny. See SearchType for all the available options
    Type        SearchType
    // the minimum size of the searched file
    MinSize     uint64
    // the maximum size of the searched fil
    MaxSize     uint64
    // part of a file name (if type is SearchAny), part of a directory name
    // (if type is SearchAny or SearchFolder) or a TTH (if type is SearchTTH)
    Query       string
}

type searchRequest struct {
    stype       SearchType
    query       string
    minSize     uint64
    maxSize     uint64
    isActive    bool
}

// Search starts a file search asynchronously. See SearchConf for the available options.
func (c *Client) Search(conf SearchConf) error {
    if conf.Type == SearchTTH && TTHIsValid(conf.Query) == false {
        return fmt.Errorf("invalid TTH")
    }

    if c.protoIsAdc == true {
        fields := make(map[string]string)

        // always add token even if we're not using it
        fields[adcFieldToken] = adcRandomSearchToken()

        switch conf.Type {
        case SearchAny:
            fields[adcFieldQueryAnd] = conf.Query

        case SearchDirectory:
            fields[adcFieldIsFileOrFolder] = adcSearchDirectory
            fields[adcFieldQueryAnd] = conf.Query

        case SearchTTH:
            fields[adcFieldFileTTH] = conf.Query
        }

        if conf.MaxSize != 0 {
            fields[adcFieldMaxSize] = numtoa(conf.MaxSize)
        }
        if conf.MinSize != 0 {
            fields[adcFieldMinSize] = numtoa(conf.MinSize)
        }

        requiredFeatures := make(map[string]struct{})

        // if we're passive, require that the recipient is active
        if c.conf.IsPassive == true {
            requiredFeatures["TCP4"] = struct{}{}
        }

        if len(requiredFeatures) > 0 {
            c.connHub.conn.Write(&msgAdcFSearchRequest{
                msgAdcTypeF{ SessionId: c.sessionId, RequiredFeatures: requiredFeatures },
                msgAdcKeySearchRequest{ fields },
            })

        } else {
            c.connHub.conn.Write(&msgAdcBSearchRequest{
                msgAdcTypeB{ c.sessionId },
                msgAdcKeySearchRequest{ fields },
            })
        }

    } else {
        if conf.MaxSize != 0 && conf.MinSize != 0 {
            return fmt.Errorf("max size and min size cannot be used together in NMDC")
        }

        c.connHub.conn.Write(&msgNmdcSearchRequest{
            Type: func() nmdcSearchType {
                switch conf.Type {
                case SearchAny: return nmdcSearchTypeAny
                case SearchDirectory: return nmdcSearchTypeDirectory
                }
                return nmdcSearchTypeTTH
            }(),
            MaxSize: conf.MaxSize,
            MinSize: conf.MinSize,
            Query: conf.Query,
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

func (c *Client) handleAdcSearchRequest(authorSessionId string, req *msgAdcKeySearchRequest) {
    var peer *Peer
    results,err := func() ([]interface{}, error) {
        peer = c.peerBySessionId(authorSessionId)
        if peer == nil {
            return nil, fmt.Errorf("search author not found")
        }

        if _,ok := req.Fields[adcFieldFileGroup]; ok {
            return nil, fmt.Errorf("search by type is not supported")
        }
        if _,ok := req.Fields[adcFieldFileExcludeExtens]; ok {
            return nil, fmt.Errorf("search by type is not supported")
        }
        if _,ok := req.Fields[adcFieldFileQueryOr]; ok {
            return nil, fmt.Errorf("search by query OR is not supported")
        }
        if _,ok := req.Fields[adcFieldFileExactSize]; ok {
            return nil, fmt.Errorf("search by exact size is not supported")
        }
        if _,ok := req.Fields[adcFieldFileExtension]; ok {
            return nil, fmt.Errorf("search by extension is not supported")
        }
        if _,ok := req.Fields[adcFieldIsFileOrFolder]; ok {
            if req.Fields[adcFieldIsFileOrFolder] != adcSearchDirectory {
                return nil, fmt.Errorf("search file only is not supported")
            }
        }
        if _,ok := req.Fields[adcFieldQueryAnd]; !ok {
            if _,ok := req.Fields[adcFieldFileTTH]; !ok {
                return nil, fmt.Errorf("AN or TR is required")
            }
        }

        return c.handleSearchRequest(&searchRequest{
            stype: func() SearchType {
                if _,ok := req.Fields[adcFieldFileTTH]; ok {
                    return SearchTTH
                }
                if _,ok := req.Fields[adcFieldIsFileOrFolder]; ok {
                    if req.Fields[adcFieldIsFileOrFolder] == adcSearchDirectory {
                        return SearchDirectory
                    }
                }
                return SearchAny
            }(),
            query: func() string {
                if _,ok := req.Fields[adcFieldFileTTH]; ok {
                    return req.Fields[adcFieldFileTTH]
                }
                return req.Fields[adcFieldQueryAnd]
            }(),
            minSize: func() uint64 {
                if val,ok := req.Fields[adcFieldMinSize]; ok {
                    return atoui64(val)
                }
                return 0
            }(),
            maxSize: func() uint64 {
                if val,ok := req.Fields[adcFieldMaxSize]; ok {
                    return atoui64(val)
                }
                return 0
            }(),
            isActive: (peer.IsPassive == false),
        })
    }()
    if err != nil {
        dolog(LevelDebug, "[search error] %s", err)
        return
    }

    var msgs []*msgAdcKeySearchResult
    for _,res := range results {
        fields := map[string]string{
            adcFieldUploadSlotCount: numtoa(c.conf.UploadMaxParallel),
        }

        switch o := res.(type) {
        case *shareFile:
            fields[adcFieldFilePath] = o.aliasPath
            fields[adcFieldFileTTH] = o.tth
            fields[adcFieldFileSize] = numtoa(o.size)

        case *shareDirectory:
            // if directory, add a trailing slash
            fields[adcFieldFilePath] = o.aliasPath + "/"
            fields[adcFieldFileTTH] = dirTTH
            fields[adcFieldFileSize] = numtoa(o.size)
        }

        // add token if sent by author
        if val,ok := req.Fields[adcFieldToken]; ok {
            fields[adcFieldToken] = val
        }

        msgs = append(msgs, &msgAdcKeySearchResult{ Fields: fields })
    }

    // send to peer
    if peer.IsPassive == false {
        go func() {
            conn,err := net.Dial("udp", fmt.Sprintf("%s:%d", peer.Ip, peer.adcUdpPort))
            if err != nil {
                return
            }
            defer conn.Close()

            for _,msg := range msgs {
                encmsg := &msgAdcUSearchResult{
                    msgAdcTypeU{ peer.adcClientId },
                    *msg,
                }
                conn.Write([]byte(encmsg.AdcTypeEncode(encmsg.AdcKeyEncode())))
            }
        }()

    // send to hub
    } else {
        for _,msg := range msgs {
            c.connHub.conn.Write(&msgAdcDSearchResult{
                msgAdcTypeD{ c.sessionId, peer.adcSessionId },
                *msg,
            })
        }
    }
}

func (c *Client) handleNmdcSearchRequest(req *msgNmdcSearchRequest) {
    results,err := func() ([]interface{}, error) {
        // we do not support search by type
        if _,ok := map[nmdcSearchType]struct{}{
            nmdcSearchTypeAny: struct{}{},
            nmdcSearchTypeDirectory: struct{}{},
            nmdcSearchTypeTTH: struct{}{},
        }[req.Type]; !ok {
            return nil, fmt.Errorf("unsupported search type: %v", req.Type)
        }
        if req.Type == nmdcSearchTypeTTH && strings.HasPrefix(req.Query, "TTH:") == false {
            return nil, fmt.Errorf("invalid TTH: %v", req.Query)
        }

        return c.handleSearchRequest(&searchRequest{
            stype: func() SearchType {
                switch req.Type {
                case nmdcSearchTypeAny: return SearchAny
                case nmdcSearchTypeDirectory: return SearchDirectory
                }
                return SearchTTH
            }(),
            query: func() string {
                if req.Type == nmdcSearchTypeTTH {
                    return req.Query[4:]
                }
                return req.Query
            }(),
            minSize: req.MinSize,
            maxSize: req.MaxSize,
            isActive: req.IsActive,
        })
    }()
    if err != nil {
        dolog(LevelDebug, "[search error] %s", err)
        return
    }

    var msgs []*msgNmdcSearchResult
    for _,res := range results {
        msgs = append(msgs, &msgNmdcSearchResult{
            Path: func() string {
                if f,ok := res.(*shareFile); ok {
                    return f.aliasPath
                }
                return res.(*shareDirectory).aliasPath
            }(),
            IsDir: func() bool {
                _,ok := res.(*shareDirectory)
                return ok
            }(),
            Size: func() uint64 {
                if f,ok := res.(*shareFile); ok {
                    return f.size
                }
                return 0
            }(),
            TTH: func() string {
                if f,ok := res.(*shareFile); ok {
                    return f.tth
                }
                return ""
            }(),
            Nick: c.conf.Nick,
            SlotAvail: c.uploadSlotAvail,
            SlotCount: c.conf.UploadMaxParallel,
            HubIp: c.hubSolvedIp,
            HubPort: c.hubPort,
        })
    }

    // send to peer
    if req.IsActive == true {
        go func() {
            conn,err := net.Dial("udp", fmt.Sprintf("%s:%d", req.Ip, req.UdpPort))
            if err != nil {
                return
            }
            defer conn.Close()

            for _,msg := range msgs {
                conn.Write([]byte(msg.NmdcEncode()))
            }
        }()

    // send to hub
    } else {
        for _,msg := range msgs {
            msg.TargetNick = req.Nick
            c.connHub.conn.Write(msg)
        }
    }
}

func (c *Client) handleSearchRequest(req *searchRequest) ([]interface{},error) {
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
                for fname,file := range dir.files {
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

            for sname,sdir := range dir.dirs {
                scanDir(sname, sdir, dirAddToResults)
            }
        }

    // search file by TTH
    } else {
        scanDir = func(dname string, dir *shareDirectory, dirAddToResults bool) {
            for _,file := range dir.files {
                if file.tth == req.query {
                    results = append(results, file)
                }
            }
            for sname,sdir := range dir.dirs {
                scanDir(sname, sdir, false)
            }
        }
    }

    // start searching
    for alias,dir := range c.shareTree {
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

    dolog(LevelInfo, "[search req] %+v | sent %d results", req, len(results))
    return results, nil
}

func (c *Client) handleSearchResult(sr *SearchResult) {
    dolog(LevelInfo, "[search res] %+v", sr)
    if c.OnSearchResult != nil {
        c.OnSearchResult(sr)
    }
}
