package dctoolkit

import (
    "net"
    "fmt"
    "time"
    "io"
    "regexp"
    "compress/zlib"
)

var errorArgsFormat = fmt.Errorf("not formatted correctly")

var reNmdcCommand = regexp.MustCompile("^\\$([a-zA-Z0-9]+)( ([^|]+))?$")
var reNmdcPublicChat = regexp.MustCompile("^<("+reStrNick+")> ([^|]+)$")
var reNmdcPrivateChat = regexp.MustCompile("^\\$To: ("+reStrNick+") From: ("+reStrNick+") \\$<("+reStrNick+")> ([^|]+)$")

type msgDecodable interface{}
type msgEncodable interface {}

type protocolNetReader struct { net.Conn }

// we provide a io.ByteReader interface to net.Conn
// otherwise zlib.NewReader() adds a bufio layer, resulting in a constant
// 4096-bytes request to Read(), that messes up the zlib on/off phase
// https://golang.org/src/compress/flate/inflate.go -> makeReader()
func (nr protocolNetReader) ReadByte() (byte, error) {
    var dest [1]byte
    _,err := nr.Read(dest[:])
    return dest[0], err
}

// this is like bufio.ReadSlice(), except it does not buffer
// anything, to allow the zlib on/off phase
// and it also strip the delimiter
func readStringUntilDelim(in io.Reader, delim byte) (string,error) {
    var buffer [10 * 1024]byte // max message size
    offset := 0
    for {
        // read one character at a time
        read,err := in.Read(buffer[offset:offset+1])
        if read == 0 {
            return "",err
        }
        offset++

        if buffer[offset-1] == delim {
            return string(buffer[:offset-1]), nil
        }

        if offset >= len(buffer) {
            return "", fmt.Errorf("message buffer exhausted")
        }
    }
}

type protocol struct {
    proto               string
    remoteLabel         string
    sendChan            chan msgEncodable
    terminated          bool
    writeTimeout        time.Duration
    binaryMode          bool
    netReadWriter       protocolNetReader
    zlibReader          io.ReadCloser
    activeReader        io.Reader
    zlibWriter          *zlib.Writer
    activeWriter        io.Writer
    writerJoined        chan struct{}
}

func newProtocol(nconn net.Conn, proto string, remoteLabel string,
    readTimeout time.Duration, writeTimeout time.Duration) *protocol {
    c := &protocol{
        proto: proto,
        remoteLabel: remoteLabel,
        writeTimeout: writeTimeout,
        writerJoined: make(chan struct{}),
        binaryMode: true,
        netReadWriter: protocolNetReader{nconn},
    }
    c.activeReader = c.netReadWriter
    c.activeWriter = c.netReadWriter
    c.SetBinaryMode(false)
    return c
}

func (c *protocol) terminate() {
    if c.terminated == true {
        return
    }
    c.terminated = true
    c.netReadWriter.Close()

    if c.binaryMode == false {
        close(c.sendChan)
        <- c.writerJoined
    }
}

func (c *protocol) Send(msg msgEncodable) {
    c.sendChan <- msg
}

func (c *protocol) SetBinaryMode(val bool) {
    if val == c.binaryMode {
        return
    }
    c.binaryMode = val

    if val == true {
        close(c.sendChan) // join writer
        <- c.writerJoined

    } else {
        c.sendChan = make(chan msgEncodable)
        go c.writer()
    }
}

func (c *protocol) SetReadCompressionOn() error {
    if c.activeReader == c.zlibReader {
        return fmt.Errorf("zlib already activated")
    }

    if c.zlibReader == nil {
        var err error
        c.zlibReader,err = zlib.NewReader(c.netReadWriter)
        if err != nil {
            panic(err)
        }
    } else {
        c.zlibReader.(zlib.Resetter).Reset(c.netReadWriter, nil)
    }
    c.activeReader = c.zlibReader

    dolog(LevelDebug, "[read zlib on]")
    return nil
}

func (c *protocol) SetWriteCompression(val bool) {
    if (val && c.activeWriter == c.zlibWriter) ||
        (!val && c.activeWriter != c.zlibWriter) {
        return
    }

    if val == true {
        c.zlibWriter = zlib.NewWriter(c.netReadWriter)
        c.activeWriter = c.zlibWriter
        dolog(LevelDebug, "[write zlib on]")
    } else {
        c.zlibWriter.Close()
        c.activeWriter = c.netReadWriter
        dolog(LevelDebug, "[write zlib off]")
    }
}

func (c *protocol) writer() {
    for {
        msg,ok := <- c.sendChan
        if ok == false {
            break // send has been closed
        }

        dolog(LevelDebug, "[c->%s] %T %+v", c.remoteLabel, msg, msg)

        encoded := func() []byte {
            if c.proto == "adc" {
                adc,ok := msg.(msgAdcTypeKeyEncodable)
                if !ok {
                    panic("command not fit for adc")
                }
                ret := []byte(adc.AdcTypeEncode(adc.AdcKeyEncode()))
                fmt.Println(string(ret))
                return ret

            } else {
                nmdc,ok := msg.(msgNmdcEncodable)
                if !ok {
                    panic("command not fit for nmdc")
                }
                return nmdc.NmdcEncode()
            }
        }()

        // do not handle errors here
        c.WriteBinary(encoded)
    }
    c.writerJoined <- struct{}{}
}

func (c *protocol) WriteBinary(in []byte) error {
    if c.writeTimeout > 0 {
        if err := c.netReadWriter.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
            return err
        }
    }
    _,err := c.activeWriter.Write(in)
    if err != nil {
        return err
    }
    return nil
}

func (c *protocol) Receive() (msgDecodable,error) {
    // Terminate() was called in a previous run
    if c.terminated == true {
        return nil, errorTerminated
    }

    // message mode
    if c.binaryMode == false {
        // adc
        if c.proto == "adc" {
            for {
                msgStr,err := readStringUntilDelim(c.activeReader, '\n')
                if err != nil {
                    // zlib EOF: disable and read again
                    if c.activeReader == c.zlibReader && err == io.EOF {
                        dolog(LevelDebug, "[read zlib off]")
                        c.zlibReader.Close()
                        c.activeReader = c.netReadWriter
                        continue
                    }
                    if c.terminated == true {
                        return nil, errorTerminated
                    }
                    return nil, err
                }

                if len(msgStr) < 5 {
                    return nil, fmt.Errorf("message too short: %s", msgStr)
                }

                msg := func() msgAdcTypeKeyDecodable {
                    switch msgStr[:4] {
                    case "BINF": return &msgAdcBInfos{}
                    case "BMSG": return &msgAdcBMessage{}
                    case "BSCH": return &msgAdcBSearchRequest{}
                    case "DMSG": return &msgAdcDMessage{}
                    case "ICMD": return &msgAdcICommand{}
                    case "IGPA": return &msgAdcIGetPass{}
                    case "IINF": return &msgAdcIInfos{}
                    case "IQUI": return &msgAdcIQuit{}
                    case "ISID": return &msgAdcISessionId{}
                    case "ISTA": return &msgAdcIStatus{}
                    case "ISUP": return &msgAdcISupports{}
                    }
                    return nil
                }()
                if msg == nil {
                    return nil, fmt.Errorf("unrecognized command: %s", msgStr)
                }

                n,err := msg.AdcTypeDecode(msgStr)
                if err != nil {
                    return nil, fmt.Errorf("unable to decode command type: %s", msgStr)
                }

                err = msg.AdcKeyDecode(msgStr[n:])
                if err != nil {
                    return nil, fmt.Errorf("unable to decode command key. type: %s key: %s err: %s",
                        msgStr[:n], msgStr[n:], err)
                }

                dolog(LevelDebug, "[%s->c] %T %+v", c.remoteLabel, msg, msg)
                return msg, nil
            }

        // nmdc
        } else {
            for {
                msgStr,err := readStringUntilDelim(c.activeReader, '|')
                if err != nil {
                    // zlib EOF: disable and read again
                    if c.activeReader == c.zlibReader && err == io.EOF {
                        dolog(LevelDebug, "[read zlib off]")
                        c.zlibReader.Close()
                        c.activeReader = c.netReadWriter
                        continue
                    }
                    if c.terminated == true {
                        return nil, errorTerminated
                    }
                    return nil, err
                }

                var msg msgDecodable

                if len(msgStr) == 0 { // empty message: skip
                    continue

                } else if matches := reNmdcCommand.FindStringSubmatch(msgStr); matches != nil {
                    key, args := matches[1], matches[3]

                    cmd := func() msgNmdcCommandDecodable {
                        switch key {
                        case "ADCGET": return &msgNmdcAdcGet{}
                        case "ADCSND": return &msgNmdcAdcSnd{}
                        case "BotList": return &msgNmdcBotList{}
                        case "ConnectToMe": return &msgNmdcConnectToMe{}
                        case "Direction": return &msgNmdcDirection{}
                        case "Error": return &msgNmdcError{}
                        case "ForceMove": return &msgNmdcForceMove{}
                        case "GetPass": return &msgNmdcGetPass{}
                        case "Hello": return &msgNmdcHello{}
                        case "HubName": return &msgNmdcHubName{}
                        case "HubTopic": return &msgNmdcHubTopic{}
                        case "Key": return &msgNmdcKey{}
                        case "Lock": return &msgNmdcLock{}
                        case "LogedIn": return &msgNmdcLoggedIn{}
                        case "MaxedOut": return &msgNmdcMaxedOut{}
                        case "MyINFO": return &msgNmdcMyInfo{}
                        case "MyNick": return &msgNmdcMyNick{}
                        case "OpList": return &msgNmdcOpList{}
                        case "Quit": return &msgNmdcQuit{}
                        case "RevConnectToMe": return &msgNmdcRevConnectToMe{}
                        case "Search": return &msgNmdcSearchRequest{}
                        case "SR": return &msgNmdcSearchResult{}
                        case "Supports": return &msgNmdcSupports{}
                        case "UserCommand": return &msgNmdcUserCommand{}
                        case "UserIP": return &msgNmdcUserIp{}
                        case "ZOn": return &msgNmdcZon{}
                        }
                        return nil
                    }()
                    if cmd == nil {
                        return nil, fmt.Errorf("unrecognized command: %s", msgStr)
                    }

                    err := cmd.NmdcDecode(args)
                    if err != nil {
                        return nil, fmt.Errorf("unable to decode arguments for %s: %s", key, err)
                    }
                    msg = cmd

                } else if matches := reNmdcPublicChat.FindStringSubmatch(msgStr); matches != nil {
                    msg = &msgNmdcPublicChat{ Author: matches[1], Content: matches[2] }

                } else if matches := reNmdcPrivateChat.FindStringSubmatch(msgStr); matches != nil {
                    msg = &msgNmdcPrivateChat{ Author: matches[3], Content: matches[4] }

                } else {
                    return nil, fmt.Errorf("Unable to parse: %s", msgStr)
                }

                dolog(LevelDebug, "[%s->c] %T %+v", c.remoteLabel, msg, msg)
                return msg, nil
            }
        }

    // binary mode
    } else {
        var buf [2048]byte
        for {
            read,err := c.activeReader.Read(buf[:])
            if read == 0 {
                // zlib EOF: disable and read again
                if c.activeReader == c.zlibReader && err == io.EOF {
                    dolog(LevelDebug, "[read zlib off]")
                    c.zlibReader.Close()
                    c.activeReader = c.netReadWriter
                    continue
                }
                if c.terminated == true {
                    return nil, errorTerminated
                }
                return nil, err
            }
            return &msgNmdcBinary{ Content: buf[:read] }, nil
        }
    }
}
