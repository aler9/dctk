package dctoolkit

import (
    "fmt"
    "net"
    "crypto/tls"
    crand "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "encoding/pem"
    "math/big"
)

type listenerTcp struct {
    client          *Client
    isEncrypted     bool
    state           string
    listener        net.Listener
}

func newListenerTcp(client *Client, isEncrypted bool) error {
    var listener net.Listener
    if isEncrypted == true {
        var err error
        priv,err := rsa.GenerateKey(crand.Reader, 1024)
        if err != nil {
            return err
        }

    	serialNumber,err := crand.Int(crand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    	if err != nil {
            return err
    	}

        template := x509.Certificate{
            SerialNumber: serialNumber,
        }
        cbytes,err := x509.CreateCertificate(crand.Reader, &template, &template, &priv.PublicKey, priv)
        if err != nil {
            return err
        }

        certPEMBlock := pem.EncodeToMemory(&pem.Block{ Type: "CERTIFICATE", Bytes: cbytes })
        keyPEMBlock := pem.EncodeToMemory(&pem.Block{ Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv) })

        cert,err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
        if err != nil {
            return err
        }

        listener,err = tls.Listen("tcp", fmt.Sprintf(":%d", client.conf.TcpTlsPort),
            &tls.Config{Certificates: []tls.Certificate{ cert }})
        if err != nil {
            return err
        }

    } else {
        var err error
        listener,err = net.Listen("tcp", fmt.Sprintf(":%d", client.conf.TcpPort))
        if err != nil {
            return err
        }
    }

    l := &listenerTcp{
        client: client,
        isEncrypted: isEncrypted,
        state: "running",
        listener: listener,
    }
    if isEncrypted == true {
        client.tcpTlsListener = l
    } else {
        client.listenerTcp = l
    }
    return nil
}

func (t *listenerTcp) terminate() {
    switch t.state {
    case "terminated":
        return

    case "running":
        t.listener.Close()

    default:
        panic(fmt.Errorf("Terminate() unsupported in state '%s'", t.state))
    }
    t.state = "terminated"
}

func (t *listenerTcp) do() {
    defer t.client.wg.Done()

    err := func() error {
        for {
            rawconn,err := t.listener.Accept()
            if err != nil {
                t.client.Safe(func() {
                    if t.state == "terminated" {
                        err = errorTerminated
                    }
                })
                return err
            }

            t.client.Safe(func() {
                newConnPeer(t.client, t.isEncrypted, true, rawconn, "", 0)
            })
        }
    }()

    t.client.Safe(func() {
        switch t.state {
        case "terminated":

        default:
            dolog(LevelInfo, "ERR: %s", err)
        }
    })
}
