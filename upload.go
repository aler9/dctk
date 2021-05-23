package dctk

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/protoadc"
	"github.com/aler9/dctk/pkg/tiger"
)

var errorNoSlots = fmt.Errorf("no slots available")

type upload struct {
	client             *Client
	terminateRequested bool
	state              string
	pconn              *peerConn
	reader             io.ReadCloser
	isCompressed       bool
	query              string
	start              uint64
	length             uint64
	offset             uint64
	lastPrintTime      time.Time
}

func (*upload) isTransfer() {}

func newUpload(client *Client,
	pconn *peerConn,
	reqQuery string,
	reqStart uint64,
	reqLength int64,
	reqCompressed bool) bool {
	u := &upload{
		client:       client,
		state:        "processing",
		pconn:        pconn,
		query:        reqQuery,
		start:        reqStart,
		isCompressed: (!client.conf.PeerDisableCompression && reqCompressed),
	}

	log.Log(client.conf.LogLevel, log.LevelInfo, "[upload] [%s] request %s (s=%d l=%d)",
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

		if !strings.HasPrefix(u.query, "file TTH/") && !strings.HasPrefix(u.query, "tthl TTH/") {
			return fmt.Errorf("invalid query")
		}

		// skip "file TTH/" or "tthl TTH/"
		tthString := u.query[9:]

		// upload is file by TTH or its tthl
		tth, err := tiger.HashFromBase32(tthString)
		if err != nil {
			return err
		}

		sfile := func() (ret *shareFile) {
			var scanDir func(dir *shareDirectory) bool
			scanDir = func(dir *shareDirectory) bool {
				for _, file := range dir.files {
					if file.tth == tth {
						ret = file
						return true
					}
				}
				for _, sdir := range dir.dirs {
					if scanDir(sdir) {
						return true
					}
				}
				return false
			}
			for _, dir := range u.client.shareTree {
				if scanDir(dir) {
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
			buf := bytes.NewBuffer(nil)
			for _, leaf := range sfile.tthl {
				buf.Write(leaf[:])
			}
			u.reader = ioutil.NopCloser(buf)
			u.length = uint64(buf.Len())
			return nil
		}

		// open file
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

		maxLength := sfile.size - u.start
		if reqLength != -1 {
			// check required length
			if uint64(reqLength) > maxLength {
				f.Close()
				return fmt.Errorf("length too big")
			}
			u.length = uint64(reqLength)
		} else {
			// set real length
			u.length = maxLength
		}

		u.reader = f
		return nil
	}()
	if err != nil {
		log.Log(u.client.conf.LogLevel, log.LevelInfo, "[peer] cannot start upload: %s", err)
		if err == errorNoSlots {
			if u.client.protoIsAdc() {
				u.pconn.conn.Write(&protoadc.AdcCStatus{ //nolint:govet
					&adc.ClientPacket{},
					&adc.Status{
						Sev:  adc.Recoverable,
						Code: protoadc.AdcCodeSlotsFull,
						Msg:  "Slots full",
					},
				})
			} else {
				u.pconn.conn.Write(&nmdc.MaxedOut{})
			}
		} else {
			if u.client.protoIsAdc() {
				u.pconn.conn.Write(&protoadc.AdcCStatus{ //nolint:govet
					&adc.ClientPacket{},
					&adc.Status{
						Sev:  adc.Recoverable,
						Code: protoadc.AdcCodeFileNotAvailable,
						Msg:  "File Not Available",
					},
				})
			} else {
				u.pconn.conn.Write(&nmdc.Error{Err: fmt.Errorf("File Not Available")})
			}
		}
		return false
	}

	if u.client.protoIsAdc() {
		queryParts := strings.Split(u.query, " ")
		u.pconn.conn.Write(&protoadc.AdcCSendFile{ //nolint:govet
			&adc.ClientPacket{},
			&adc.GetResponse{
				Type:       queryParts[0],
				Path:       queryParts[1],
				Start:      int64(u.start),
				Bytes:      int64(u.length),
				Compressed: u.isCompressed,
			},
		})

	} else {
		queryParts := strings.Split(u.query, " ")
		u.pconn.conn.Write(&nmdc.ADCSnd{
			ContentType: nmdc.String(queryParts[0]),
			Identifier:  nmdc.String(queryParts[1]),
			Start:       u.start,
			Length:      u.length,
			Compressed:  u.isCompressed,
		})
	}

	client.transfers[u] = struct{}{}
	u.client.uploadSlotAvail--
	u.pconn.state = "delegated_upload"
	u.pconn.transfer = u
	return true
}

func (u *upload) Close() {
	if u.terminateRequested {
		return
	}
	u.terminateRequested = true
	u.pconn.close()
}

func (u *upload) handleUpload() error {
	u.pconn.conn.SetSyncMode(true)
	if u.isCompressed {
		u.pconn.conn.EnableWriterZlib()
	}

	u.lastPrintTime = time.Now()
	buf := make([]byte, 1024*1024)
	bufLength := uint64(len(buf))

	for {
		// apply length
		maxLength := func() uint64 {
			if (u.offset + bufLength) >= u.length {
				return u.length - u.offset
			}
			return bufLength
		}()
		if maxLength == 0 {
			break
		}

		n, err := u.reader.Read(buf[:maxLength])
		if err != nil && err != io.EOF {
			return err
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
			log.Log(u.client.conf.LogLevel, log.LevelInfo, "[sent] %d/%d (%.1f KiB/s)", u.offset, u.length, speed)
		}
	}

	if u.isCompressed {
		u.pconn.conn.DisableWriterZlib()
	}
	u.pconn.conn.SetSyncMode(false)

	return nil
}

func (u *upload) handleExit(err error) {
	if !u.terminateRequested && err != nil {
		log.Log(u.client.conf.LogLevel, log.LevelInfo, "ERR (upload) [%s]: %s", u.pconn.peer.Nick, err)
	}

	delete(u.client.transfers, u)

	u.reader.Close()

	u.client.uploadSlotAvail++

	if err == nil {
		log.Log(u.client.conf.LogLevel, log.LevelInfo, "[upload] [%s] finished %s (s=%d l=%d)",
			u.pconn.peer.Nick, dcReadableQuery(u.query), u.start, u.length)
	} else {
		log.Log(u.client.conf.LogLevel, log.LevelInfo, "[upload] [%s] failed %s",
			u.pconn.peer.Nick, dcReadableQuery(u.query))
	}
}
