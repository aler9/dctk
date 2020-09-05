package dctoolkit

import (
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"

	"github.com/aler9/dctoolkit/proto"
)

type listenerTcp struct {
	client             *Client
	isEncrypted        bool
	terminateRequested bool
	listener           net.Listener
}

func newListenerTcp(client *Client, isEncrypted bool) error {
	var listener net.Listener
	if isEncrypted == true {
		var err error
		priv, err := rsa.GenerateKey(crand.Reader, 1024)
		if err != nil {
			return err
		}

		serialNumber, err := crand.Int(crand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return err
		}

		template := x509.Certificate{
			SerialNumber: serialNumber,
		}
		bcert, err := x509.CreateCertificate(crand.Reader, &template, &template, &priv.PublicKey, priv)
		if err != nil {
			return err
		}

		if client.protoIsAdc() {
			xcert, err := x509.ParseCertificate(bcert)
			if err != nil {
				return err
			}
			client.adcFingerprint = proto.AdcCertFingerprint(xcert)
		}

		certPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: bcert})
		keyPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

		tcert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			return err
		}

		listener, err = tls.Listen("tcp4", fmt.Sprintf(":%d", client.conf.TcpTlsPort),
			&tls.Config{Certificates: []tls.Certificate{tcert}})
		if err != nil {
			return err
		}

	} else {
		var err error
		listener, err = net.Listen("tcp4", fmt.Sprintf(":%d", client.conf.TcpPort))
		if err != nil {
			return err
		}
	}

	l := &listenerTcp{
		client:      client,
		isEncrypted: isEncrypted,
		listener:    listener,
	}
	if isEncrypted == true {
		client.tcpTlsListener = l
	} else {
		client.listenerTcp = l
	}
	return nil
}

func (t *listenerTcp) close() {
	if t.terminateRequested == true {
		return
	}
	t.terminateRequested = true
	t.listener.Close()
}

func (t *listenerTcp) do() {
	defer t.client.wg.Done()

	for {
		rawconn, err := t.listener.Accept()
		// listener closed
		if err != nil {
			break
		}

		t.client.Safe(func() {
			newPeerConn(t.client, t.isEncrypted, true, rawconn, "", 0, "")
		})
	}
}