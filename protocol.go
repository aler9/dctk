package dctoolkit

import (
    "net"
    "fmt"
    "time"
    "regexp"
    "io"
    "compress/zlib"
)

var reCommand = regexp.MustCompile("^\\$([a-zA-Z0-9]+)( ([^|]+))?\\|$")
var rePublicChat = regexp.MustCompile("^<("+reStrNick+")> ([^|]+)\\|$")
var rePrivateChat = regexp.MustCompile("^\\$To: ("+reStrNick+") From: ("+reStrNick+") \\$<("+reStrNick+")> ([^|]+)|$")

type msgBase interface {
    Bytes()     []byte
}

type msgCommand struct {
    Key         string
    Args        string
}

func (c msgCommand) Bytes() []byte {
    return []byte(fmt.Sprintf("$%s %s|", c.Key, c.Args))
}

type msgPublicChat struct {
    Author      string
    Content     string
}

func (c msgPublicChat) Bytes() []byte {
    return []byte(fmt.Sprintf("<%s> %s|", c.Author, c.Content))
}

type msgPrivateChat struct {
    Author      string
    Dest        string
    Content     string
}

func (c msgPrivateChat) Bytes() []byte {
    return []byte(fmt.Sprintf("$To: %s From: %s $<%s> %s|", c.Dest, c.Author, c.Author, c.Content))
}

type msgBinary struct {
    Content     []byte
}

func (c msgBinary) Bytes() []byte {
    return c.Content
}

type protocolNetConn struct { net.Conn }

// we also provide a io.ByteReader interface to zlib.NewReader()
// otherwise a bufio layer is added, resulting in a constant 4096-bytes request
// to Read(), that messes up the zlib on/off phase
// https://golang.org/src/compress/flate/inflate.go -> makeReader()
func (nr protocolNetConn) ReadByte() (byte, error) {
    var dest [1]byte
    _,err := nr.Read(dest[:])
    return dest[0], err
}

type Protocol struct {
    nconn               protocolNetConn
    remoteLabel         string
    Send                chan msgBase
    terminated          bool
    ChatAllowed         bool
    writeTimeout        time.Duration
    zlibReader          io.ReadCloser
    msgBuffer           [1024 * 10]byte // max message size
    msgOffset           int
    writerJoined        chan struct{}
    zlibWriter          *zlib.Writer
    activeReceiver      func() (msgBase,error)
    activeReader        io.Reader
    activeWriter        io.Writer
    binaryMode          bool
}

func newProtocol(nconn net.Conn, remoteLabel string, readTimeout time.Duration, writeTimeout time.Duration) *Protocol {
    c := &Protocol{
        nconn: protocolNetConn{nconn},
        remoteLabel: remoteLabel,
        writeTimeout: writeTimeout,
        writerJoined: make(chan struct{}),
        binaryMode: true,
    }
    c.activeReader = nconn
    c.activeWriter = nconn
    c.SetBinaryMode(false)
    return c
}

func (c *Protocol) terminate() {
    if c.terminated == true {
        return
    }
    c.terminated = true
    c.nconn.Close()

    if c.binaryMode == false {
        close(c.Send)
        <- c.writerJoined
    }
}

func (c *Protocol) SetBinaryMode(val bool) {
    if val == c.binaryMode {
        return
    }
    c.binaryMode = val

    if val == true {
        c.activeReceiver = c.receiveBinary
        close(c.Send) // join writer
        <- c.writerJoined

    } else {
        c.activeReceiver = c.receiveMessage
        c.Send = make(chan msgBase)
        go c.writer()
    }
}

func (c *Protocol) SetReadCompressionOn() error {
    if c.activeReader == c.zlibReader {
        return fmt.Errorf("zlib already activated")
    }

    var err error
    c.zlibReader,err = zlib.NewReader(c.nconn)
    if err != nil {
        panic(err)
    }
    c.activeReader = c.zlibReader

    dolog(LevelDebug, "[read zlib on]")
    return nil
}

func (c *Protocol) SetWriteCompression(val bool) {
    if (val && c.activeWriter == c.zlibWriter) ||
        (!val && c.activeWriter != c.zlibWriter) {
        return
    }

    if val == true {
        c.zlibWriter = zlib.NewWriter(c.nconn)
        c.activeWriter = c.zlibWriter
        dolog(LevelDebug, "[write zlib on]")
    } else {
        c.zlibWriter.Close()
        c.activeWriter = c.nconn
        dolog(LevelDebug, "[write zlib off]")
    }
}

func (c *Protocol) writer() {
    for {
        command,ok := <- c.Send
        if ok == false {
            break // Send has been closed
        }

        if m,ok := command.(msgCommand); ok {
            dolog(LevelDebug, "[c->%s] %s %s", c.remoteLabel, m.Key, m.Args)
        }
        msg := command.Bytes()

        // do not handle errors here
        c.WriteBinary(msg)
    }
    c.writerJoined <- struct{}{}
}

func (c *Protocol) WriteBinary(in []byte) error {
    if c.writeTimeout > 0 {
        if err := c.nconn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
            return err
        }
    }
    _,err := c.activeWriter.Write(in)
    if err != nil {
        return err
    }
    return nil
}

func (c *Protocol) Receive() (msgBase,error) {
    // Terminate() was called in a previous run
    if c.terminated == true {
        return nil, errorTerminated
    }
    return c.activeReceiver()
}

func (c *Protocol) receiveMessage() (msgBase,error) {
    for {
        if c.msgOffset >= len(c.msgBuffer) {
            return nil, fmt.Errorf("message buffer exhausted")
        }

        // read one character at a time
        read,err := c.activeReader.Read(c.msgBuffer[c.msgOffset:c.msgOffset+1])
        if read == 0 {
            // zlib EOF: disable and read again
            if c.activeReader == c.zlibReader && err == io.EOF {
                dolog(LevelDebug, "[read zlib off]")
                c.zlibReader.Close()
                c.activeReader = c.nconn
                continue
            }
            if c.terminated == true {
                return nil, errorTerminated
            }
            return nil, err
        }
        c.msgOffset++

        if c.msgBuffer[c.msgOffset-1] == '|' {
            msgStr := string(c.msgBuffer[:c.msgOffset])
            c.msgOffset = 0

            if len(msgStr) == 1 { // empty message: skip
                continue

            } else if matches := reCommand.FindStringSubmatch(msgStr); matches != nil {
                msg := msgCommand{ Key: matches[1], Args: matches[3] }
                dolog(LevelDebug, "[%s->c] %s %s", c.remoteLabel, msg.Key, msg.Args)
                return msg, nil

            } else if c.ChatAllowed == true {
                if matches := rePublicChat.FindStringSubmatch(msgStr); matches != nil {
                    msg := msgPublicChat{ Author: matches[1], Content: matches[2] }
                    dolog(LevelInfo, "[PUB] <%s> %s", msg.Author, msg.Content)
                    return msg, nil

                } else if matches := rePrivateChat.FindStringSubmatch(msgStr); matches != nil {
                    msg := msgPrivateChat{ Author: matches[3], Content: matches[4] }
                    dolog(LevelInfo, "[PRIV] <%s> %s", msg.Author, msg.Content)
                    return msg, nil
                }
            }
            return nil, fmt.Errorf("Unable to parse: %s", msgStr)
        }
    }
}

func (c *Protocol) receiveBinary() (msgBase,error) {
    var buf [2048]byte
    for {
        read,err := c.activeReader.Read(buf[:])
        if read == 0 {
            // zlib EOF: disable and read again
            if c.activeReader == c.zlibReader && err == io.EOF {
                dolog(LevelDebug, "[read zlib off]")
                c.zlibReader.Close()
                c.activeReader = c.nconn
                continue
            }
            if c.terminated == true {
                return nil, errorTerminated
            }
            return nil, err
        }
        return msgBinary{ Content: buf[:read] }, nil
    }
}
