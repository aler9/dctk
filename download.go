package dctoolkit

import (
	"bytes"
	"compress/bzip2"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// DownloadConf allows to configure a download.
type DownloadConf struct {
	// the peer from which downloading
	Peer *Peer
	// the TTH of the file to download
	TTH string
	// the starting point of the file part to download, in bytes
	Start uint64
	// the length of the file part. Leave zero to download the entire file
	Length int64
	// if filled, the file is saved on the desired path on disk, otherwise it is kept on RAM
	SavePath string
	// after download, do not attempt to validate the file through its TTH
	SkipValidation bool

	filelist bool
}

// Download represents an in-progress file download.
type Download struct {
	conf          DownloadConf
	client        *Client
	state         string
	wakeUp        chan struct{}
	pconn         *connPeer
	query         string
	adcToken      string
	writer        io.WriteCloser
	content       []byte
	offset        uint64
	length        uint64
	lastPrintTime time.Time
}

func (*Download) isTransfer() {}

// DownloadCount returns the number of remaining downloads, queued or active.
func (c *Client) DownloadCount() int {
	count := 0
	for t := range c.transfers {
		if _, ok := t.(*Download); ok {
			count++
		}
	}
	return count
}

func (c *Client) downloadByAdcToken(adcToken string) *Download {
	for t := range c.transfers {
		if dl, ok := t.(*Download); ok {
			if dl.adcToken == adcToken && dl.state == "waiting_peer" {
				return dl
			}
		}
	}
	return nil
}

func (c *Client) downloadPendingByPeer(peer *Peer) *Download {
	dl, ok := c.activeDownloadsByPeer[peer.Nick]
	if ok && dl.state == "waiting_peer" {
		return dl
	}
	return nil
}

// DownloadFileList starts downloading the file list of a given peer.
func (c *Client) DownloadFileList(peer *Peer, savePath string) (*Download, error) {
	return c.DownloadFile(DownloadConf{
		Peer:     peer,
		SavePath: savePath,
		filelist: true,
	})
}

// DownloadFLFile starts downloading a file given a file list entry.
func (c *Client) DownloadFLFile(peer *Peer, file *FileListFile, savePath string) (*Download, error) {
	return c.DownloadFile(DownloadConf{
		Peer:     peer,
		TTH:      file.TTH,
		SavePath: savePath,
	})
}

// DownloadFLDirectory starts downloading recursively all the files
// inside a file list directory.
func (c *Client) DownloadFLDirectory(peer *Peer, dir *FileListDirectory, savePath string) error {
	var dlDir func(sdir *FileListDirectory, dpath string) error
	dlDir = func(sdir *FileListDirectory, dpath string) error {
		// create destionation directory if does not exist
		os.Mkdir(dpath, 0755)

		for _, file := range sdir.Files {
			_, err := c.DownloadFLFile(peer, file, filepath.Join(dpath, file.Name))
			if err != nil {
				return err
			}
		}
		for _, ssdir := range sdir.Dirs {
			err := dlDir(ssdir, filepath.Join(dpath, ssdir.Name))
			if err != nil {
				return err
			}
		}
		return nil
	}
	return dlDir(dir, savePath)
}

// DownloadFile starts downloading a file by its Tiger Tree Hash (TTH). See DownloadConf for the options.
func (c *Client) DownloadFile(conf DownloadConf) (*Download, error) {
	if conf.Length <= 0 {
		conf.Length = -1
	}
	if conf.filelist == false && TTHIsValid(conf.TTH) == false {
		return nil, fmt.Errorf("invalid TTH")
	}

	d := &Download{
		conf:   conf,
		client: c,
		wakeUp: make(chan struct{}, 1),
		state:  "uninitialized",
	}
	d.client.transfers[d] = struct{}{}

	// build query
	d.query = func() string {
		if d.conf.filelist == true {
			return "file files.xml.bz2"
		}
		return "file TTH/" + d.conf.TTH
	}()

	dolog(LevelInfo, "[download] [%s] request %s (s=%d l=%d)",
		d.conf.Peer.Nick, dcReadableQuery(d.query), d.conf.Start, d.conf.Length)

	d.client.wg.Add(1)
	go d.do()
	return d, nil
}

// Conf returns the configuration passed at download initialization.
func (d *Download) Conf() DownloadConf {
	return d.conf
}

// Content returns the downloaded file content ONLY if SavePath is not used, otherwise
// file content is saved directly on disk
func (d *Download) Content() []byte {
	return d.content
}

func (d *Download) terminate() {
	switch d.state {
	case "terminated":
		return

	case "waiting_activedl", "waiting_slot", "waiting_peer":
		d.wakeUp <- struct{}{}

	case "waited_activedl", "waited_slot", "waited_peer":

	case "processing":
		d.pconn.terminate()

	default:
		panic(fmt.Errorf("Terminate() unsupported in state '%s'", d.state))
	}
	d.state = "terminated"
}

func (d *Download) do() {
	defer d.client.wg.Done()

	err := func() error {
		for {
			safeState, err := func() (string, error) {
				d.client.mutex.Lock()
				defer d.client.mutex.Unlock()

				for {
					switch d.state {
					case "terminated":
						return "", errorTerminated

					case "uninitialized":
						if _, ok := d.client.activeDownloadsByPeer[d.conf.Peer.Nick]; ok {
							d.state = "waiting_activedl"
						} else {
							d.state = "waited_activedl"
							continue
						}

					case "waited_activedl":
						d.client.activeDownloadsByPeer[d.conf.Peer.Nick] = d
						if d.client.downloadSlotAvail <= 0 {
							d.state = "waiting_slot"
						} else {
							d.state = "waited_slot"
							continue
						}

					case "waited_slot":
						d.client.downloadSlotAvail -= 1
						if pconn, ok := d.client.connPeersByKey[nickDirectionPair{d.conf.Peer.Nick, "download"}]; !ok {
							dolog(LevelDebug, "[download] [%s] requesting new connection", d.conf.Peer.Nick)

							// generate new token
							if d.client.protoIsAdc == true {
								d.adcToken = adcRandomToken()
							}

							d.client.peerRequestConnection(d.conf.Peer, d.adcToken)
							d.state = "waiting_peer"

						} else {
							dolog(LevelDebug, "[download] [%s] using existing connection", d.conf.Peer.Nick)
							pconn.state = "delegated_download"
							pconn.transfer = d
							d.pconn = pconn
							d.state = "waited_peer"
							continue
						}

					case "waited_peer":
						dolog(LevelInfo, "[download] [%s] processing", d.conf.Peer.Nick)
						d.state = "processing"
					}
					break
				}
				return d.state, nil
			}()

			switch safeState {
			case "":
				return err

			case "waiting_activedl", "waiting_slot":
				<-d.wakeUp

			case "waiting_peer":
				timeout := time.NewTimer(10 * time.Second)
				select {
				case <-timeout.C:
					return fmt.Errorf("download timed out")
				case <-d.wakeUp:
				}

			case "processing":
				if d.client.protoIsAdc == true {
					d.pconn.conn.Write(&msgAdcCGetFile{
						msgAdcTypeC{},
						msgAdcKeyGetFile{
							Query:  d.query,
							Start:  d.conf.Start,
							Length: d.conf.Length,
							Compressed: (d.client.conf.PeerDisableCompression == false &&
								(d.conf.Length <= 0 || d.conf.Length >= (1024*10))),
						},
					})

				} else {
					d.pconn.conn.Write(&msgNmdcGetFile{
						Query:  d.query,
						Start:  d.conf.Start,
						Length: d.conf.Length,
						Compressed: (d.client.conf.PeerDisableCompression == false &&
							(d.conf.Length <= 0 || d.conf.Length >= (1024*10))),
					})
				}

				// exit this routine and do the work in the peer routine
				return nil
			}
		}
	}()

	if err != nil {
		d.client.Safe(func() {
			d.handleExit(err)
		})
	}
}

func (d *Download) handleSendFile(reqQuery string, reqStart uint64,
	reqLength uint64, reqCompressed bool) error {

	if reqQuery != d.query {
		return fmt.Errorf("filename returned by client is wrong: %s vs %s", reqQuery, d.query)
	}
	if reqStart != d.conf.Start {
		return fmt.Errorf("peer returned wrong start: %d instead of %d", reqStart, d.conf.Start)
	}
	if reqCompressed == true && d.client.conf.PeerDisableCompression == true {
		return fmt.Errorf("compression is active but is disabled")
	}

	if d.conf.Length == -1 {
		d.length = reqLength
	} else {
		d.length = uint64(d.conf.Length)
		if d.length != reqLength {
			return fmt.Errorf("peer returned wrong length: %d instead of %d", d.length, reqLength)
		}
	}

	if d.length == 0 {
		return fmt.Errorf("downloading null files is not supported")
	}

	d.pconn.conn.SetReadBinary(true)
	if reqCompressed == true {
		d.pconn.conn.SetReadCompressionOn()
	}

	// save in file
	if d.conf.SavePath != "" {
		f, err := os.Create(d.conf.SavePath + ".tmp")
		if err != nil {
			return fmt.Errorf("unable to create destination file")
		}
		d.writer = f

		// save in ram
	} else {
		d.content = make([]byte, d.length)
		d.writer = newBytesWriteCloser(d.content)
	}

	return nil
}

func (d *Download) handleDownload(msgi msgDecodable) error {
	switch msg := msgi.(type) {
	case *msgAdcCStatus:
		return fmt.Errorf("error: %+v", msg)

	case *msgAdcCSendFile:
		return d.handleSendFile(msg.Query, msg.Start, msg.Length, msg.Compressed)

	case *msgNmdcMaxedOut:
		return fmt.Errorf("maxed out")

	case *msgNmdcError:
		return fmt.Errorf("error: %s", msg.Error)

	case *msgNmdcSendFile:
		return d.handleSendFile(msg.Query, msg.Start, msg.Length, msg.Compressed)

	case *msgBinary:
		newLength := d.offset + uint64(len(msg.Content))
		if newLength > d.length {
			return fmt.Errorf("binary content too long (%d)", newLength)
		}

		_, err := d.writer.Write(msg.Content)
		if err != nil {
			d.writer.Close()
			return err
		}
		d.offset = newLength

		if time.Since(d.lastPrintTime) >= (1 * time.Second) {
			d.lastPrintTime = time.Now()
			dolog(LevelInfo, "[recv] %d/%d", d.offset, d.length)
		}

		if d.offset == d.length {
			d.pconn.conn.SetReadBinary(false)
			d.writer.Close()

			// file list: unzip in final path
			if d.conf.filelist {
				if d.conf.SavePath != "" {
					srcf, err := os.Open(d.conf.SavePath + ".tmp")
					if err != nil {
						return err
					}

					destf, err := os.Create(d.conf.SavePath)
					if err != nil {
						srcf.Close()
						return err
					}

					_, err = io.Copy(destf, bzip2.NewReader(srcf))
					srcf.Close()
					destf.Close()
					if err != nil {
						return err
					}

					if err := os.Remove(d.conf.SavePath + ".tmp"); err != nil {
						return err
					}

				} else {
					cnt, err := ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(d.content)))
					if err != nil {
						return err
					}
					d.content = cnt
				}

				// normal file
			} else {
				// validate
				if d.conf.SkipValidation == false && d.conf.Start == 0 && d.conf.Length <= 0 {
					dolog(LevelInfo, "[download] [%s] validating", d.conf.Peer.Nick)

					// file in disk
					var contentTTH string
					if d.conf.SavePath != "" {
						var err error
						contentTTH, err = TTHFromFile(d.conf.SavePath + ".tmp")
						if err != nil {
							return err
						}

						// file in ram
					} else {
						contentTTH = TTHFromBytes(d.content)
					}

					if contentTTH != d.conf.TTH {
						return fmt.Errorf("validation failed")
					}
				}

				// move to final path
				if d.conf.SavePath != "" {
					if err := os.Rename(d.conf.SavePath+".tmp", d.conf.SavePath); err != nil {
						return err
					}
				}
			}

			return errorTerminated
		}

	default:
		return fmt.Errorf("unhandled: %T %+v", msgi, msgi)
	}
	return nil
}

func (d *Download) handleExit(err error) {
	switch d.state {
	case "terminated":
	case "success":
	default:
		dolog(LevelInfo, "ERR (download) [%s]: %s", d.conf.Peer.Nick, err)
	}

	delete(d.client.transfers, d)

	// free activedl and unlock next download
	delete(d.client.activeDownloadsByPeer, d.conf.Peer.Nick)
	for rot := range d.client.transfers {
		if od, ok := rot.(*Download); ok {
			if od.state == "waiting_activedl" && d.conf.Peer == od.conf.Peer {
				od.state = "waited_activedl"
				od.wakeUp <- struct{}{}
				break
			}
		}
	}

	// free slot and unlock next download
	d.client.downloadSlotAvail += 1
	for rot := range d.client.transfers {
		if od, ok := rot.(*Download); ok {
			if od.state == "waiting_slot" {
				od.state = "waited_slot"
				od.wakeUp <- struct{}{}
				break
			}
		}
	}

	// call callbacks
	if d.state == "success" {
		dolog(LevelInfo, "[download] [%s] finished %s (s=%d l=%d)",
			d.conf.Peer.Nick, dcReadableQuery(d.query), d.conf.Start, len(d.content))
		if d.client.OnDownloadSuccessful != nil {
			d.client.OnDownloadSuccessful(d)
		}
	} else {
		dolog(LevelInfo, "[download] [%s] failed %s", d.conf.Peer.Nick, dcReadableQuery(d.query))
		if d.client.OnDownloadError != nil {
			d.client.OnDownloadError(d)
		}
	}
}
