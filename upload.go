package dctoolkit

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

var errorNoSlots = fmt.Errorf("no slots available")

type upload struct {
	client        *Client
	state         string
	pconn         *connPeer
	reader        io.ReadCloser
	compressed    bool
	query         string
	start         uint64
	length        uint64
	offset        uint64
	lastPrintTime time.Time
}

func (*upload) isTransfer() {}

func newUpload(client *Client, pconn *connPeer, reqQuery string, reqStart uint64,
	reqLength int64, reqCompressed bool) {

	u := &upload{
		client: client,
		state:  "processing",
		pconn:  pconn,
		query:  reqQuery,
		start:  reqStart,
		compressed: (client.conf.PeerDisableCompression == false &&
			reqCompressed == true),
	}

	dolog(LevelInfo, "[upload] [%s] request %s (s=%d l=%d)",
		pconn.peer.Nick, dcReadableQuery(u.query), u.start, reqLength)

	err := func() error {
		// check available slots
		if u.client.uploadSlotAvail <= 0 {
			return errorNoSlots
		}

		// upload is file list
		if u.query == "file files.xml.bz2" {
			if u.start != 0 || reqLength != -1 {
				return fmt.Errorf("filelist seeking is not supported")
			}

			u.reader = ioutil.NopCloser(bytes.NewReader(u.client.fileList))
			u.length = uint64(len(u.client.fileList))
			return nil
		}

		// upload is file by TTH or its tthl
		sfile := func() (ret *shareFile) {
			msgTTH := u.query[9:] // skip "file TTH/" or "tthl TTH/"
			var scanDir func(dir *shareDirectory) bool
			scanDir = func(dir *shareDirectory) bool {
				for _, file := range dir.files {
					if string(file.tth) == msgTTH {
						ret = file
						return true
					}
				}
				for _, sdir := range dir.dirs {
					if scanDir(sdir) == true {
						return true
					}
				}
				return false
			}
			for _, dir := range u.client.shareTree {
				if scanDir(dir) == true {
					break
				}
			}
			return
		}()
		if sfile == nil {
			return fmt.Errorf("file does not exists")
		}

		// upload is file tthl
		if strings.HasPrefix(u.query, "tthl") {
			if u.start != 0 || reqLength != -1 {
				return fmt.Errorf("tthl seeking is not supported")
			}
			u.reader = ioutil.NopCloser(bytes.NewReader(sfile.tthl))
			u.length = uint64(len(sfile.tthl))
			return nil
		}

		// open file
		var err error
		var f *os.File
		f, err = os.Open(sfile.realPath)
		if err != nil {
			return err
		}

		// apply start
		_, err = f.Seek(int64(u.start), 0)
		if err != nil {
			f.Close()
			return err
		}

		// set real length
		maxLength := sfile.size - u.start
		if reqLength != -1 {
			if uint64(reqLength) > maxLength {
				f.Close()
				return fmt.Errorf("length too big")
			}
			u.length = uint64(reqLength)
		} else {
			u.length = maxLength
		}

		u.reader = f
		return nil
	}()
	if err != nil {
		dolog(LevelInfo, "[peer] cannot start upload: %s", err)
		if err == errorNoSlots {
			if u.client.protoIsAdc == true {
				u.pconn.conn.Write(&msgAdcCStatus{
					msgAdcTypeC{},
					msgAdcKeyStatus{
						Type:    adcStatusWarning,
						Code:    adcCodeSlotsFull,
						Message: "Slots full",
					},
				})
			} else {
				u.pconn.conn.Write(&msgNmdcMaxedOut{})
			}
		} else {
			if u.client.protoIsAdc == true {
				u.pconn.conn.Write(&msgAdcCStatus{
					msgAdcTypeC{},
					msgAdcKeyStatus{
						Type:    adcStatusWarning,
						Code:    adcCodeFileNotAvailable,
						Message: "File Not Available",
					},
				})
			} else {
				u.pconn.conn.Write(&msgNmdcError{Error: "File Not Available"})
			}
		}
		return
	}

	if u.client.protoIsAdc == true {
		u.pconn.conn.Write(&msgAdcCSendFile{
			msgAdcTypeC{},
			msgAdcKeySendFile{
				Query:      u.query,
				Start:      u.start,
				Length:     u.length,
				Compressed: u.compressed,
			},
		})

	} else {
		u.pconn.conn.Write(&msgNmdcSendFile{
			Query:      u.query,
			Start:      u.start,
			Length:     u.length,
			Compressed: u.compressed,
		})
	}

	client.transfers[u] = struct{}{}
	u.client.uploadSlotAvail -= 1
	u.pconn.state = "delegated_upload"
	u.pconn.transfer = u
}

func (u *upload) terminate() {
	switch u.state {
	case "terminated":
		return

	case "processing":
		u.pconn.terminate()

	default:
		panic(fmt.Errorf("terminate() unsupported in state '%s'", u.state))
	}
	u.state = "terminated"
}

func (u *upload) handleUpload() error {
	u.pconn.conn.SetSyncMode(true)
	if u.compressed == true {
		u.pconn.conn.SetWriteCompression(true)
	}

	// setup time to correctly compute speed
	u.lastPrintTime = time.Now()

	var buf [1024 * 1024]byte
	for {
		n, err := u.reader.Read(buf[:])
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		u.offset += uint64(n)

		err = u.pconn.conn.WriteSync(buf[:n])
		if err != nil {
			return err
		}

		since := time.Since(u.lastPrintTime)
		if since >= (1 * time.Second) {
			u.lastPrintTime = time.Now()
			speed := float64(u.pconn.conn.PullWriteCounter()) / 1024 / (float64(since) / float64(time.Second))
			dolog(LevelInfo, "[sent] %d/%d (%.1f KiB/s)", u.offset, u.length, speed)
		}
	}

	if u.compressed == true {
		u.pconn.conn.SetWriteCompression(false)
	}
	u.pconn.conn.SetSyncMode(false)

	return nil
}

func (u *upload) handleExit(err error) {
	switch u.state {
	case "terminated":
	case "success":
	default:
		dolog(LevelInfo, "ERR (upload) [%s]: %s", u.pconn.peer.Nick, err)
	}

	delete(u.client.transfers, u)

	u.reader.Close()

	u.client.uploadSlotAvail += 1

	if u.state == "success" {
		dolog(LevelInfo, "[upload] [%s] finished %s (s=%d l=%d)",
			u.pconn.peer.Nick, dcReadableQuery(u.query), u.start, u.length)
	} else {
		dolog(LevelInfo, "[upload] [%s] failed %s",
			u.pconn.peer.Nick, dcReadableQuery(u.query))
	}
}
