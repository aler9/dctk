package dctoolkit

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"
)

var errorDelegatedUpload = fmt.Errorf("delegated upload")

type nickDirectionPair struct {
	nick      string
	direction string
}

type connPeer struct {
	client             *Client
	isEncrypted        bool
	isActive           bool
	terminateRequested bool
	terminate          chan struct{}
	state              string
	conn               protocol
	tlsConn            *tls.Conn
	adcToken           string
	passiveIp          string
	passivePort        uint
	peer               *Peer
	remoteLock         []byte
	localDirection     string
	localBet           uint
	remoteDirection    string
	remoteBet          uint
	direction          string
	transfer           transfer
}

func newConnPeer(client *Client, isEncrypted bool, isActive bool,
	rawconn net.Conn, ip string, port uint, adcToken string) *connPeer {
	p := &connPeer{
		client:      client,
		isEncrypted: isEncrypted,
		isActive:    isActive,
		terminate:   make(chan struct{}, 1),
		adcToken:    adcToken,
	}
	p.client.connPeers[p] = struct{}{}

	if isActive == true {
		dolog(LevelInfo, "[peer] incoming %s%s", rawconn.RemoteAddr(), func() string {
			if p.isEncrypted == true {
				return " (secure)"
			}
			return ""
		}())
		p.state = "connected"
		if p.isEncrypted == true {
			p.tlsConn = rawconn.(*tls.Conn)
		}
		if client.protoIsAdc == true {
			p.conn = newProtocolAdc("p", rawconn, true, true)
		} else {
			p.conn = newProtocolNmdc("p", rawconn, true, true)
		}
	} else {
		dolog(LevelInfo, "[peer] outgoing %s:%d%s", ip, port, func() string {
			if p.isEncrypted == true {
				return " (secure)"
			}
			return ""
		}())
		p.state = "connecting"
		p.passiveIp = ip
		p.passivePort = port
	}

	p.client.wg.Add(1)
	go p.do()
	return p
}

func (p *connPeer) close() {
	if p.terminateRequested == true {
		return
	}
	p.terminateRequested = true
	p.terminate <- struct{}{}
}

func (p *connPeer) do() {
	defer p.client.wg.Done()

	err := func() error {
		// connect to peer
		connect := false
		p.client.Safe(func() {
			if p.state == "connecting" {
				connect = true
			}
		})
		if connect == true {
			ce := newConnEstablisher(
				fmt.Sprintf("%s:%d", p.passiveIp, p.passivePort),
				10*time.Second, 3)

			select {
			case <-p.terminate:
				return errorTerminated
			case <-ce.Wait:
			}

			if ce.Error != nil {
				return ce.Error
			}

			rawconn := ce.Conn
			if p.isEncrypted == true {
				p.tlsConn = tls.Client(rawconn, &tls.Config{InsecureSkipVerify: true})
				rawconn = p.tlsConn
			}

			if p.client.protoIsAdc == true {
				p.conn = newProtocolAdc("p", rawconn, true, true)
			} else {
				p.conn = newProtocolNmdc("p", rawconn, true, true)
			}

			p.client.Safe(func() {
				p.state = "connected"
			})

			dolog(LevelInfo, "[peer] connected %s%s", rawconn.RemoteAddr(),
				func() string {
					if p.isEncrypted == true {
						return " (secure)"
					}
					return ""
				}())

			// if transfer is passive, we are the first to talk
			if p.client.protoIsAdc == true {
				p.conn.Write(&msgAdcCSupports{
					msgAdcTypeC{},
					msgAdcKeySupports{map[string]struct{}{
						adcFeatureBas0:         {},
						adcFeatureBase:         {},
						adcFeatureTiger:        {},
						adcFeatureFileListBzip: {},
						adcFeatureZlibGet:      {},
					}},
				})

			} else {
				p.conn.Write(&msgNmdcMyNick{Nick: p.client.conf.Nick})
				p.conn.Write(&msgNmdcLock{
					Lock: "EXTENDEDPROTOCOLABCABCABCABCABCABC",
					Pk:   p.client.conf.PkValue,
					Ref:  fmt.Sprintf("%s:%d", p.client.hubSolvedIp, p.client.hubPort),
				})
			}
		}

		readDone := make(chan error)
		go func() {
			readDone <- func() error {
				for {
					msg, err := p.conn.Read()
					if err != nil {
						return err
					}

					p.client.Safe(func() {
						// pre-transfer
						if p.state != "delegated_download" {
							err = p.handleMessage(msg)

							// download
						} else {
							d := p.transfer.(*Download)
							err = d.handleDownload(msg)
							if err == errorTerminated {
								p.transfer = nil
								p.state = "wait_download"
								d.handleExit(nil)
								err = nil // do not close connection
							}
						}
					})

					// upload
					if err == errorDelegatedUpload {
						u := p.transfer.(*upload)

						err := u.handleUpload()
						if err != nil {
							return err
						}

						p.client.Safe(func() {
							p.transfer = nil
							p.state = "wait_upload"
							u.handleExit(nil)
						})

					} else if err != nil {
						return err
					}
				}
			}()
		}()

		select {
		case <-p.terminate:
			p.conn.Close()
			<-readDone
			return errorTerminated

		case err := <-readDone:
			p.conn.Close()
			return err
		}
	}()

	p.client.Safe(func() {
		// transfer abruptly interrupted, doesnt care if the conn was terminated or not
		switch p.state {
		case "delegated_upload", "delegated_download":
			p.transfer.handleExit(err)
		}

		if p.terminateRequested == false {
			switch p.state {
			// timeout while waiting, not an error
			case "wait_upload", "wait_download":
			default:
				if p.terminateRequested == false {
					dolog(LevelInfo, "ERR (connPeer): %s", err)
				}
			}
		}

		if p.conn != nil {
			p.conn.Close()
		}

		delete(p.client.connPeers, p)

		if p.peer != nil && p.direction != "" {
			delete(p.client.connPeersByKey, nickDirectionPair{p.peer.Nick, p.direction})
		}

		dolog(LevelInfo, "[peer] disconnected")
	})
}

func (p *connPeer) handleMessage(msgi msgDecodable) error {
	switch msg := msgi.(type) {
	case *msgAdcCStatus:
		if msg.Type != adcStatusOk {
			return fmt.Errorf("error (%d): %s", msg.Code, msg.Message)
		}

	case *msgAdcCSupports:
		if p.state != "connected" {
			return fmt.Errorf("[Supports] invalid state: %s", p.state)
		}
		p.state = "supports"
		if p.isActive == true {
			p.conn.Write(&msgAdcCSupports{
				msgAdcTypeC{},
				msgAdcKeySupports{map[string]struct{}{
					adcFeatureBas0:         {},
					adcFeatureBase:         {},
					adcFeatureTiger:        {},
					adcFeatureFileListBzip: {},
					adcFeatureZlibGet:      {},
				}},
			})

		} else {
			p.conn.Write(&msgAdcCInfos{
				msgAdcTypeC{},
				msgAdcKeyInfos{map[string]string{
					adcFieldClientId: dcBase32Encode(p.client.clientId),
					adcFieldToken:    p.adcToken,
				}},
			})
		}

	case *msgAdcCInfos:
		if p.state != "supports" {
			return fmt.Errorf("[Infos] invalid state: %s", p.state)
		}
		p.state = "infos"

		clientId, ok := msg.Fields[adcFieldClientId]
		if ok == false {
			return fmt.Errorf("client id not provided")
		}

		p.peer = p.client.peerByClientId(dcBase32Decode(clientId))
		if p.peer == nil {
			return fmt.Errorf("unknown client id (%s)", clientId)
		}

		if p.isActive == true {
			token, ok := msg.Fields[adcFieldToken]
			if ok == false {
				return fmt.Errorf("token not provided")
			}
			p.adcToken = token

			p.conn.Write(&msgAdcCInfos{
				msgAdcTypeC{},
				msgAdcKeyInfos{map[string]string{
					adcFieldClientId: dcBase32Encode(p.client.clientId),
					// token is not sent back when in active mode
				}},
			})
		} else {
			// validate peer fingerprint
			// can be performed on client-side only since many clients do not send
			// their certificate when in passive mode
			if p.client.protoIsAdc == true && p.isEncrypted == true &&
				p.peer.adcFingerprint != "" {

				connFingerprint := adcCertificateFingerprint(
					p.tlsConn.ConnectionState().PeerCertificates[0])

				if connFingerprint != p.peer.adcFingerprint {
					return fmt.Errorf("unable to validate peer fingerprint (%s vs %s)",
						connFingerprint, p.peer.adcFingerprint)
				}
				dolog(LevelInfo, "[peer] fingerprint validated")
			}
		}

		dl := p.client.downloadByAdcToken(p.adcToken)
		if dl != nil {
			key := nickDirectionPair{p.peer.Nick, "download"}
			if _, ok := p.client.connPeersByKey[key]; ok {
				return fmt.Errorf("a connection with this peer and direction already exists")
			}
			p.client.connPeersByKey[key] = p

			p.direction = "download"
			p.state = "delegated_download"
			p.transfer = dl
			dl.pconn = p
			dl.state = "processing"
			dl.peerChan <- struct{}{}

		} else {
			key := nickDirectionPair{p.peer.Nick, "upload"}
			if _, ok := p.client.connPeersByKey[key]; ok {
				return fmt.Errorf("a connection with this peer and direction already exists")
			}
			p.client.connPeersByKey[key] = p

			p.direction = "upload"
			p.state = "wait_upload"
		}

	case *msgAdcCGetFile:
		if p.state != "wait_upload" {
			return fmt.Errorf("[AdcGet] invalid state: %s", p.state)
		}
		ok := newUpload(p.client, p, msg.Query, msg.Start, msg.Length, msg.Compressed)
		if ok {
			return errorDelegatedUpload
		}

	case *msgNmdcMyNick:
		if p.state != "connected" {
			return fmt.Errorf("[MyNick] invalid state: %s", p.state)
		}
		p.state = "mynick"
		p.peer = p.client.peerByNick(msg.Nick)
		if p.peer == nil {
			return fmt.Errorf("peer not connected to hub (%s)", msg.Nick)
		}

	case *msgNmdcLock:
		if p.state != "mynick" {
			return fmt.Errorf("[Lock] invalid state: %s", p.state)
		}
		p.state = "lock"
		p.remoteLock = []byte(msg.Lock)

		// if transfer is active, wait remote before sending MyNick and Lock
		if p.isActive {
			p.conn.Write(&msgNmdcMyNick{Nick: p.client.conf.Nick})
			p.conn.Write(&msgNmdcLock{
				Lock: "EXTENDEDPROTOCOLABCABCABCABCABCABC",
				Pk:   p.client.conf.PkValue,
			})
		}

		features := map[string]struct{}{
			nmdcFeatureMiniSlots:    {},
			nmdcFeatureFileListBzip: {},
			nmdcFeatureAdcGet:       {},
			nmdcFeatureTTHLeaves:    {},
			nmdcFeatureTTHDownload:  {},
		}
		if p.client.conf.PeerDisableCompression == false {
			features[nmdcFeatureZlibGet] = struct{}{}
		}
		p.conn.Write(&msgNmdcSupports{features})

		p.localBet = uint(randomInt(1, 0x7FFF))

		// try download
		if p.client.downloadPendingByPeer(p.peer) != nil {
			p.localDirection = "download"
			p.conn.Write(&msgNmdcDirection{
				Direction: "Download",
				Bet:       p.localBet,
			})
			// upload
		} else {
			p.localDirection = "upload"
			p.conn.Write(&msgNmdcDirection{
				Direction: "Upload",
				Bet:       p.localBet,
			})
		}

		p.conn.Write(&msgNmdcKey{Key: nmdcComputeKey(p.remoteLock)})

	case *msgNmdcSupports:
		if p.state != "lock" {
			return fmt.Errorf("[Supports] invalid state: %s", p.state)
		}
		p.state = "supports"

	case *msgNmdcDirection:
		if p.state != "supports" {
			return fmt.Errorf("[Direction] invalid state: %s", p.state)
		}
		p.state = "direction"
		p.remoteDirection = strings.ToLower(msg.Direction)
		p.remoteBet = msg.Bet

	case *msgNmdcKey:
		if p.state != "direction" {
			return fmt.Errorf("[Key] invalid state: %s", p.state)
		}
		p.state = "key"

		var direction string
		if p.localDirection == "upload" && p.remoteDirection == "download" {
			direction = "upload"

		} else if p.localDirection == "download" && p.remoteDirection == "upload" {
			direction = "download"

		} else if p.localDirection == "download" && p.remoteDirection == "download" {
			// bet win
			if p.localBet > p.remoteBet {
				direction = "download"

				// bet lost
			} else if p.localBet < p.remoteBet {
				direction = "upload"

				// if there's a pending download, request another connection
				if dl := p.client.downloadPendingByPeer(p.peer); dl != nil {
					p.client.peerRequestConnection(dl.conf.Peer, "")
				}

			} else {
				return fmt.Errorf("equal random numbers")
			}

		} else {
			return fmt.Errorf("double upload request")
		}

		key := nickDirectionPair{p.peer.Nick, direction}
		if _, ok := p.client.connPeersByKey[key]; ok {
			return fmt.Errorf("a connection with this peer and direction already exists")
		}

		p.client.connPeersByKey[key] = p
		p.direction = direction

		// upload
		if p.direction == "upload" {
			p.state = "wait_upload"

			// download
		} else {
			dl := p.client.downloadPendingByPeer(p.peer)
			if dl == nil {
				return fmt.Errorf("download connection but cannot find download")
			}

			p.state = "delegated_download"
			p.transfer = dl
			dl.pconn = p
			dl.state = "processing"
			dl.peerChan <- struct{}{}
		}

	case *msgNmdcGetFile:
		if p.state != "wait_upload" {
			return fmt.Errorf("[AdcGet] invalid state: %s", p.state)
		}
		ok := newUpload(p.client, p, msg.Query, msg.Start, msg.Length, msg.Compressed)
		if ok {
			return errorDelegatedUpload
		}

	default:
		return fmt.Errorf("unhandled: %T %+v", msgi, msgi)
	}
	return nil
}
