package dctoolkit

import (
    "fmt"
    "strings"
    "net"
    "path/filepath"
)

type SearchType int
const (
    // search for a file by name
    SearchFile      SearchType = iota
    // search for a directory by name
    SearchDirectory
    // search for a file by TTH
    SearchTTH
)

type nmdcSearchType int
const (
    nsTypeInvalid     nmdcSearchType = 0
    nsTypeAny         nmdcSearchType = 1
    nsTypeAudio       nmdcSearchType = 2
    nsTypeCompressed  nmdcSearchType = 3
    nsTypeDocument    nmdcSearchType = 4
    nsTypeExe         nmdcSearchType = 5
    nsTypePicture     nmdcSearchType = 6
    nsTypeVideo       nmdcSearchType = 7
    nsTypeDirectory   nmdcSearchType = 8
    nsTypeTTH         nmdcSearchType = 9
)

func nmdcSearchEscape(in string) string {
    return strings.Replace(in, " ", "$", -1)
}

func nmdcSearchUnescape(in string) string {
    return strings.Replace(in, "$", " ", -1)
}

type SearchResult struct {
    // whether the result is a directory
    IsDir       bool
    // path of a file or directory matching a search request
    Path        string
    // file TTH
    TTH         string
    // whether the search result was received in passive or active mode
    IsActive    bool
    // the nickname of the peer sending the result
    Nick        string
    // the currently available upload slots of the peer
    SlotAvail   uint
    // the total number of the upload slots of the peer
    SlotCount   uint
    // the hub ip
    HubIp       string
    // the hub port
    HubPort     uint
}

type SearchConf struct {
    // the search type, defaults to SearchFile. See SearchType for all the available options
    Type        SearchType
    // the minimum size of the searched file
    MinSize     uint
    // the maximum size of the searched fil
    MaxSize     uint
    // part of a file name (if type is SearchFile), part of a directory name
    // (if type is SearchFolder) or a TTH (if type is SearchTTH)
    Query       string
}

// Search starts a file search asynchronously. See SearchConf for the available options.
func (c *Client) Search(conf SearchConf) error {
    if conf.Type == SearchTTH && TTHIsValid(conf.Query) == false {
        return fmt.Errorf("invalid TTH")
    }

    if c.hubIsAdc == true {
        fields := map[string]string{
            "AN": conf.Query,
        }
        if conf.MaxSize != 0 {
            fields["LE"] = fmt.Sprintf("%d", conf.MaxSize)
        }
        if conf.MinSize != 0 {
            fields["GE"] = fmt.Sprintf("%d", conf.MinSize)
        }
        if conf.Type == SearchDirectory {
            fields["TY"] = "2"
        } else {
            fields["TY"] = "1"
        }

        c.connHub.conn.Write(&msgAdcBSearchRequest{
            msgAdcTypeB{ c.sessionId },
            msgAdcKeySearchRequest{ Fields: fields },
        })

    } else {
        if conf.MaxSize != 0 && conf.MinSize != 0 {
            return fmt.Errorf("max size and min size cannot be used together")
        }

        c.connHub.conn.Write(&msgNmdcSearchRequest{
            Type: func() nmdcSearchType {
                switch conf.Type {
                case SearchFile: return nsTypeAny
                case SearchDirectory: return nsTypeDirectory
                }
                return nsTypeTTH
            }(),
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
    }
    return nil
}

func (c *Client) onSearchRequest(req *msgNmdcSearchRequest) {
    if len(req.Query) < 3 {
        return
    }
    // we support only these nmdc types
    if _,ok := map[nmdcSearchType]struct{}{
        nsTypeAny: struct{}{},
        nsTypeDirectory: struct{}{},
        nsTypeTTH: struct{}{},
    }[req.Type]; !ok {
        return
    }
    if req.Type == nsTypeTTH &&
        (!strings.HasPrefix(req.Query, "TTH:") || !TTHIsValid(req.Query[4:])) {
        return
    }

    // normalize query
    if req.Type != nsTypeTTH {
        req.Query = strings.ToLower(req.Query)
    }

    var replies []*msgNmdcSearchResult
    var scanDir func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool)

    // search file or directory by name
    if req.Type != nsTypeTTH {
        scanDir = func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool) {
            // always add directories
            if dirAddToResults == false {
                dirAddToResults = strings.Contains(strings.ToLower(dname), req.Query)
            }
            if dirAddToResults {
                replies = append(replies, &msgNmdcSearchResult{
                    Path: filepath.Join(dpath, dname),
                    IsDir: true,
                })
            }

            // add files only if nsTypeAny
            if req.Type == nsTypeAny {
                for fname,file := range dir.files {
                    fileAddToResults := dirAddToResults
                    if fileAddToResults == false {
                        fileAddToResults = strings.Contains(strings.ToLower(fname), req.Query)
                    }
                    if fileAddToResults {
                        replies = append(replies, &msgNmdcSearchResult{
                            Path: filepath.Join(dpath, dname, fname),
                            IsDir: false,
                            TTH: file.tth,
                        })
                    }
                }
            }

            for sname,sdir := range dir.dirs {
                scanDir(filepath.Join(dpath, dname), sname, sdir, dirAddToResults)
            }
        }

    // search file by TTH
    } else {
        reqTTH := req.Query[4:]
        scanDir = func(dpath string, dname string, dir *shareDirectory, dirAddToResults bool) {
            for fname,file := range dir.files {
                if file.tth == reqTTH {
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

    if c.hubIsAdc == true {

    } else {
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
                    conn.Write(msg.NmdcEncode())
                }
            }()

        } else {
            // send to hub
            for _,msg := range replies {
                msg.TargetNick = req.Nick
                c.connHub.conn.Write(msg)
            }
        }
    }

    dolog(LevelInfo, "[search req] %+v | sent %d results", req, len(replies))
}
