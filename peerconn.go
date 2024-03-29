package dctk

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/aler9/go-dc/adc"
	"github.com/aler9/go-dc/nmdc"

	"github.com/aler9/dctk/pkg/log"
	"github.com/aler9/dctk/pkg/protoadc"
	"github.com/aler9/dctk/pkg/protocommon"
	"github.com/aler9/dctk/pkg/protonmdc"
)

var errorDelegatedUpload = fmt.Errorf("delegated upload")

type nickDirectionPair struct {
	nick      string
	direction string
}

type peerConn struct {
	client             *Client
	isEncrypted        bool
	isActive           bool
	terminateRequested bool
	terminate          chan struct{}
	state              string
	conn               conn
	tlsConn            *tls.Conn
	adcToken           string
	passiveIP          string
	passivePort        uint
	peer               *Peer
	localDirection     string
	localBet           uint
	remoteIsUpload     bool
	remoteBet          uint
	direction          string
	transfer           transfer
}

func newPeerConn(client *Client, isEncrypted bool, isActive bool,
	rawconn net.Conn, ip string, port uint, adcToken string,
) *peerConn {
	p := &peerConn{
		client:      client,
		isEncrypted: isEncrypted,
		isActive:    isActive,
		terminate:   make(chan struct{}),
		adcToken:    adcToken,
	}
	p.client.peerConns[p] = struct{}{}

	if isActive {
		log.Log(client.conf.LogLevel, log.LevelInfo, "[peer] incoming %s%s", rawconn.RemoteAddr(), func() string {
			if p.isEncrypted {
				return " (secure)"
			}
			return ""
		}())
		p.state = "connected"
		if p.isEncrypted {
			p.tlsConn = rawconn.(*tls.Conn)
		}
		if client.protoIsAdc() {
			p.conn = protoadc.NewConn(p.client.conf.LogLevel, "p", rawconn, true, true)
		} else {
			p.conn = protonmdc.NewConn(p.client.conf.LogLevel, "p", rawconn, true, true)
		}
	} else {
		log.Log(client.conf.LogLevel, log.LevelInfo, "[peer] outgoing %s:%d%s", ip, port, func() string {
			if p.isEncrypted {
				return " (secure)"
			}
			return ""
		}())
		p.state = "connecting"
		p.passiveIP = ip
		p.passivePort = port
	}

	p.client.wg.Add(1)
	go p.do()
	return p
}

func (p *peerConn) close() {
	if p.terminateRequested {
		return
	}
	p.terminateRequested = true
	close(p.terminate)
}

func (p *peerConn) do() {
	defer p.client.wg.Done()

	err := func() error {
		// connect to peer
		connect := false
		p.client.Safe(func() {
			if p.state == "connecting" {
				connect = true
			}
		})
		if connect {
			ce := newConnEstablisher(
				fmt.Sprintf("%s:%d", p.passiveIP, p.passivePort),
				10*time.Second, 3)

			select {
			case <-p.terminate:
				return protocommon.ErrorTerminated
			case <-ce.Wait:
			}

			if ce.Error != nil {
				return ce.Error
			}

			rawconn := ce.Conn
			if p.isEncrypted {
				p.tlsConn = tls.Client(rawconn, &tls.Config{InsecureSkipVerify: true})
				rawconn = p.tlsConn
			}

			if p.client.protoIsAdc() {
				p.conn = protoadc.NewConn(p.client.conf.LogLevel, "p", rawconn, true, true)
			} else {
				p.conn = protonmdc.NewConn(p.client.conf.LogLevel, "p", rawconn, true, true)
			}

			p.client.Safe(func() {
				p.state = "connected"
			})

			log.Log(p.client.conf.LogLevel, log.LevelInfo, "[peer] connected %s%s", rawconn.RemoteAddr(),
				func() string {
					if p.isEncrypted {
						return " (secure)"
					}
					return ""
				}())

			// if transfer is passive, we are the first to talk
			if p.client.protoIsAdc() {
				p.conn.Write(&protoadc.AdcCSupports{ //nolint:govet
					&adc.ClientPacket{},
					&adc.Supported{adc.ModFeatures{ //nolint:govet
						adc.FeaBAS0: true,
						adc.FeaBASE: true,
						adc.FeaTIGR: true,
						adc.FeaBZIP: true,
						adc.FeaZLIG: true,
					}},
				})
			} else {
				p.conn.Write(&nmdc.MyNick{Name: nmdc.Name(p.client.conf.Nick)})
				p.conn.Write(&nmdc.Lock{
					Lock: "EXTENDEDPROTOCOLABCABCABCABCABCABC",
					PK:   p.client.conf.PkValue,
					Ref:  fmt.Sprintf("%s:%d", p.client.hubSolvedIP, p.client.hubPort),
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
							if err == protocommon.ErrorTerminated {
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
			return protocommon.ErrorTerminated

		case err := <-readDone:
			p.conn.Close()
			return err
		}
	}()

	p.client.Safe(func() {
		if !p.terminateRequested {
			log.Log(p.client.conf.LogLevel, log.LevelInfo, "ERR (peerConn): %s", err)
		}

		// transfer abruptly interrupted, doesnt care if the conn was terminated or not
		switch p.state {
		case "delegated_upload", "delegated_download":
			p.transfer.handleExit(err)
		}

		if p.conn != nil {
			p.conn.Close()
		}

		delete(p.client.peerConns, p)

		if p.peer != nil && p.direction != "" {
			delete(p.client.peerConnsByKey, nickDirectionPair{p.peer.Nick, p.direction})
		}

		log.Log(p.client.conf.LogLevel, log.LevelInfo, "[peer] disconnected")
	})
}

func (p *peerConn) handleMessage(msgi protocommon.MsgDecodable) error {
	switch msg := msgi.(type) {
	case *protoadc.AdcCStatus:
		if msg.Msg.Sev != adc.Success {
			return fmt.Errorf("error (%d): %s", msg.Msg.Code, msg.Msg.Msg)
		}

	case *protoadc.AdcCSupports:
		if p.state != "connected" {
			return fmt.Errorf("[Supports] invalid state: %s", p.state)
		}
		p.state = "supports"
		if p.isActive {
			p.conn.Write(&protoadc.AdcCSupports{ //nolint:govet
				&adc.ClientPacket{},
				&adc.Supported{adc.ModFeatures{ //nolint:govet
					adc.FeaBAS0: true,
					adc.FeaBASE: true,
					adc.FeaTIGR: true,
					adc.FeaBZIP: true,
					adc.FeaZLIG: true,
				}},
			})
		} else {
			info := &adc.UserInfo{}
			info.Id = p.client.clientID
			info.Token = p.adcToken

			p.conn.Write(&protoadc.AdcCInfos{ //nolint:govet
				&adc.ClientPacket{},
				info,
			})
		}

	case *protoadc.AdcCInfos:
		if p.state != "supports" {
			return fmt.Errorf("[Infos] invalid state: %s", p.state)
		}
		p.state = "infos"

		p.peer = p.client.peerByClientID(msg.Msg.Id)
		if p.peer == nil {
			return fmt.Errorf("unknown client id (%s)", msg.Msg.Id)
		}

		if p.isActive {
			if msg.Msg.Token == "" {
				return fmt.Errorf("token not provided")
			}
			p.adcToken = msg.Msg.Token

			info := &adc.UserInfo{}
			info.Id = p.client.clientID
			// token is not sent back when in active mode

			p.conn.Write(&protoadc.AdcCInfos{ //nolint:govet
				&adc.ClientPacket{},
				info,
			})

			// validate peer fingerprint
			// can be performed on client-side only since many clients do not send
			// their certificate when in passive mode
		} else if p.client.protoIsAdc() && p.isEncrypted &&
			p.peer.adcFingerprint != "" {
			connFingerprint := protoadc.AdcCertFingerprint(
				p.tlsConn.ConnectionState().PeerCertificates[0])

			if connFingerprint != p.peer.adcFingerprint {
				return fmt.Errorf("unable to validate peer fingerprint (%s vs %s)",
					connFingerprint, p.peer.adcFingerprint)
			}
			log.Log(p.client.conf.LogLevel, log.LevelInfo, "[peer] fingerprint validated")
		}

		dl := p.client.downloadByAdcToken(p.adcToken)
		if dl != nil {
			key := nickDirectionPair{p.peer.Nick, "download"}
			if _, ok := p.client.peerConnsByKey[key]; ok {
				return fmt.Errorf("a connection with this peer and direction already exists")
			}
			p.client.peerConnsByKey[key] = p

			p.direction = "download"
			p.state = "delegated_download"
			p.transfer = dl
			dl.pconn = p
			dl.state = "processing"
			dl.peerChan <- struct{}{}
		} else {
			key := nickDirectionPair{p.peer.Nick, "upload"}
			if _, ok := p.client.peerConnsByKey[key]; ok {
				return fmt.Errorf("a connection with this peer and direction already exists")
			}
			p.client.peerConnsByKey[key] = p

			p.direction = "upload"
			p.state = "wait_upload"
		}

	case *protoadc.AdcCGetFile:
		if p.state != "wait_upload" {
			return fmt.Errorf("[AdcGet] invalid state: %s", p.state)
		}
		query := msg.Msg.Type + " " + msg.Msg.Path
		ok := newUpload(p.client, p, query, uint64(msg.Msg.Start),
			msg.Msg.Bytes, msg.Msg.Compressed)
		if ok {
			return errorDelegatedUpload
		}

	case *nmdc.MyNick:
		if p.state != "connected" {
			return fmt.Errorf("[MyNick] invalid state: %s", p.state)
		}
		p.state = "mynick"
		p.peer = p.client.peerByNick(string(msg.Name))
		if p.peer == nil {
			return fmt.Errorf("peer not connected to hub (%s)", msg.Name)
		}

	case *nmdc.Lock:
		if p.state != "mynick" {
			return fmt.Errorf("[Lock] invalid state: %s", p.state)
		}
		p.state = "lock"

		// if transfer is active, wait remote before sending MyNick and Lock
		if p.isActive {
			p.conn.Write(&nmdc.MyNick{Name: nmdc.Name(p.client.conf.Nick)})
			p.conn.Write(&nmdc.Lock{
				Lock: "EXTENDEDPROTOCOLABCABCABCABCABCABC",
				PK:   p.client.conf.PkValue,
			})
		}

		features := []string{
			nmdc.ExtMinislots,
			nmdc.ExtXmlBZList,
			nmdc.ExtADCGet,
			nmdc.ExtTTHL,
			nmdc.ExtTTHF,
		}
		if !p.client.conf.PeerDisableCompression {
			features = append(features, nmdc.ExtZLIG)
		}
		p.conn.Write(&nmdc.Supports{features}) //nolint:govet

		p.localBet = uint(randomInt(1, 0x7FFF))

		// try download
		if p.client.downloadPendingByPeer(p.peer) != nil {
			p.localDirection = "download"
			p.conn.Write(&nmdc.Direction{
				Upload: false,
				Number: p.localBet,
			})
			// upload
		} else {
			p.localDirection = "upload"
			p.conn.Write(&nmdc.Direction{
				Upload: true,
				Number: p.localBet,
			})
		}

		p.conn.Write(msg.Key())

	case *nmdc.Supports:
		if p.state != "lock" {
			return fmt.Errorf("[Supports] invalid state: %s", p.state)
		}
		p.state = "supports"

	case *nmdc.Direction:
		if p.state != "supports" {
			return fmt.Errorf("[Direction] invalid state: %s", p.state)
		}
		p.state = "direction"
		p.remoteIsUpload = msg.Upload
		p.remoteBet = msg.Number

	case *nmdc.Key:
		if p.state != "direction" {
			return fmt.Errorf("[Key] invalid state: %s", p.state)
		}
		p.state = "key"

		var direction string

		switch {
		case p.localDirection == "upload" && !p.remoteIsUpload:
			direction = "upload"

		case p.localDirection == "download" && p.remoteIsUpload:
			direction = "download"

		case p.localDirection == "download" && !p.remoteIsUpload:
			switch {
			// bet win
			case p.localBet > p.remoteBet:
				direction = "download"

			// bet lost
			case p.localBet < p.remoteBet:
				direction = "upload"

				// if there's a pending download, request another connection
				if dl := p.client.downloadPendingByPeer(p.peer); dl != nil {
					p.client.peerRequestConnection(dl.conf.Peer, "")
				}

			default:
				return fmt.Errorf("equal random numbers")
			}

		default:
			return fmt.Errorf("double upload request")
		}

		key := nickDirectionPair{p.peer.Nick, direction}
		if _, ok := p.client.peerConnsByKey[key]; ok {
			return fmt.Errorf("a connection with this peer and direction already exists")
		}

		p.client.peerConnsByKey[key] = p
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

	case *nmdc.ADCGet:
		if p.state != "wait_upload" {
			return fmt.Errorf("[AdcGet] invalid state: %s", p.state)
		}
		query := string(msg.ContentType) + " " + string(msg.Identifier)
		ok := newUpload(p.client, p, query, msg.Start, msg.Length, msg.Compressed)
		if ok {
			return errorDelegatedUpload
		}

	default:
		return fmt.Errorf("unhandled: %T %+v", msgi, msgi)
	}
	return nil
}
