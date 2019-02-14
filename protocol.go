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

type protocolTimedNetReadWriter struct {
    in              net.Conn
    readTimeout     time.Duration
    writeTimeout    time.Duration
}

func (nr protocolTimedNetReadWriter) Close() {
    nr.in.Close()
}

func (nr protocolTimedNetReadWriter) Read(buf []byte) (int, error) {
    if nr.readTimeout > 0 {
        if err := nr.in.SetReadDeadline(time.Now().Add(nr.readTimeout)); err != nil {
            return 0, err
        }
    }
    return nr.in.Read(buf)
}

func (nr protocolTimedNetReadWriter) Write(buf []byte) (int, error) {
    if nr.writeTimeout > 0 {
        if err := nr.in.SetWriteDeadline(time.Now().Add(nr.writeTimeout)); err != nil {
            return 0, err
        }
    }
    return nr.in.Write(buf)
}

// we provide a io.ByteReader interface, otherwise zlib.NewReader()
// adds a bufio layer, resulting in a constant 4096-bytes request to Read(),
// that messes up the zlib on/off phase.
// https://golang.org/src/compress/flate/inflate.go -> makeReader()
// this could be replaced by a bufio wrapper, but is IMHO less efficients
func (nr protocolTimedNetReadWriter) ReadByte() (byte, error) {
    var dest [1]byte
    _,err := nr.Read(dest[:])
    return dest[0], err
}

// this is like bufio.ReadSlice(), except it does not buffer
// anything, to allow the zlib on/off phase
// and it also strips the delimiter
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
    isAdc               bool
    remoteLabel         string
    sendChan            chan msgEncodable
    terminated          bool
    binaryMode          bool
    netReadWriter       protocolTimedNetReadWriter
    zlibReader          io.ReadCloser
    activeReader        io.Reader
    zlibWriter          *zlib.Writer
    activeWriter        io.Writer
    writerJoined        chan struct{}
}

func newProtocol(nconn net.Conn, isAdc bool, remoteLabel string,
    readTimeout time.Duration, writeTimeout time.Duration) *protocol {
    c := &protocol{
        isAdc: isAdc,
        remoteLabel: remoteLabel,
        writerJoined: make(chan struct{}),
        binaryMode: true,
        netReadWriter: protocolTimedNetReadWriter{
            in: nconn,
            readTimeout: readTimeout,
            writeTimeout: writeTimeout,
        },
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
            if c.isAdc == true {
                adc,ok := msg.(msgAdcTypeKeyEncodable)
                if !ok {
                    panic("command not fit for adc")
                }
                ret := []byte(adc.AdcTypeEncode(adc.AdcKeyEncode()))
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
        if c.isAdc == true {
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

                if msgStr[4] != ' ' {
                    return nil, fmt.Errorf("invalid message: %s", msgStr)
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

                n,err := msg.AdcTypeDecode(msgStr[5:])
                if err != nil {
                    return nil, fmt.Errorf("unable to decode command type: %s", msgStr)
                }

                err = msg.AdcKeyDecode(msgStr[5+n:])
                if err != nil {
                    return nil, fmt.Errorf("unable to decode command key. type: %s key: %s err: %s",
                        msgStr[:5+n], msgStr[5+n:], err)
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
