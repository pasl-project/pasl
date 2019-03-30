package ecdh

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"io"
	"math/big"
)

type ellipticECDH struct {
	ECDH
	curve elliptic.Curve
}

// NewEllipticECDH creates a new instance of ECDH with the given elliptic.Curve curve
// to use as the elliptical curve for elliptical curve diffie-hellman.
func NewEllipticECDH(curve elliptic.Curve) ECDH {
	return &ellipticECDH{
		curve: curve,
	}
}

func (e *ellipticECDH) GenerateKey(rand io.Reader) (crypto.PrivateKey, crypto.PublicKey, error) {
	var priv *ecdsa.PrivateKey
	var pub *ecdsa.PublicKey
	var err error

	priv, err = ecdsa.GenerateKey(e.curve, rand)
	if err != nil {
		return nil, nil, err
	}
	pub = &priv.PublicKey

	return priv, pub, nil
}

func (e *ellipticECDH) Marshal(p crypto.PublicKey) []byte {
	pub := p.(*ecdsa.PublicKey)
	return elliptic.Marshal(e.curve, pub.X, pub.Y)
}

func (e *ellipticECDH) Unmarshal(data []byte) (crypto.PublicKey, bool) {
	var key *ecdsa.PublicKey
	var x, y *big.Int

	x, y = elliptic.Unmarshal(e.curve, data)
	if x == nil || y == nil {
		return key, false
	}
	key = &ecdsa.PublicKey{
		Curve: e.curve,
		X:     x,
		Y:     y,
	}
	return key, true
}

// GenerateSharedSecret takes in a public key and a private key
// and generates a shared secret.
//
// RFC5903 Section 9 states we should only return x.
func (e *ellipticECDH) GenerateSharedSecret(privKey crypto.PrivateKey, pubKey crypto.PublicKey) ([]byte, error) {
	priv := privKey.(*ecdsa.PrivateKey)
	pub := pubKey.(*ecdsa.PublicKey)

	x, _ := e.curve.ScalarMult(pub.X, pub.Y, priv.D.Bytes())
	return x.Bytes(), nil
}
