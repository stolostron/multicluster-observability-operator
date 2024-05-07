package certificates

import (
	"bytes"
	"time"

	"github.com/openshift/library-go/pkg/crypto"
)

func NewSigningCertKeyPair(signerName string, validity time.Duration) (certData, keyData []byte, err error) {
	ca, err := crypto.MakeSelfSignedCAConfigForDuration(signerName, validity)
	if err != nil {
		return nil, nil, err
	}

	certBytes := &bytes.Buffer{}
	keyBytes := &bytes.Buffer{}
	if err := ca.WriteCertConfig(certBytes, keyBytes); err != nil {
		return nil, nil, err
	}

	return certBytes.Bytes(), keyBytes.Bytes(), nil
}
