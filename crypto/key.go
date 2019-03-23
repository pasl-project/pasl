/*
PASL - Personalized Accounts & Secure Ledger

Copyright (C) 2018 PASL Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/coin-network/curve"
	"github.com/pasl-project/pasl/utils"
)

const (
	NIDsecp256k1 uint16 = 714
	NIDsecp521r1        = 716
	NIDsect283k1        = 729
	NIDsecp384r1        = 715
)

type Public struct {
	TypeId uint16
	ecdsa.PublicKey
	utils.Serializable
}

type PublicSerialized struct {
	TypeId uint16
	X      []byte
	Y      []byte
}

type PublicSerializedPlain struct {
	TypeId uint16
	X      utils.Serializable
	Y      utils.Serializable
}

type Signature struct {
	R *big.Int
	S *big.Int
}

type SignatureSerialized struct {
	R []byte
	S []byte
}

type Key struct {
	Private []byte
	Public  *Public
}

func (this *SignatureSerialized) Decompress() *Signature {
	return &Signature{
		R: (&big.Int{}).SetBytes(this.R),
		S: (&big.Int{}).SetBytes(this.S),
	}
}

func CurveById(typeId uint16) (elliptic.Curve, error) {
	switch typeId {
	case NIDsecp256k1:
		return curve.S256(), nil
	case NIDsect283k1:
		return Sect283k1(), nil
	case NIDsecp521r1:
		return elliptic.P521(), nil
	case NIDsecp384r1:
		return elliptic.P384(), nil
	}
	return nil, fmt.Errorf("Unknown curve id %d", typeId)
}

func NewKey(typeId uint16) (*Key, error) {
	curve, err := CurveById(typeId)
	if err != nil {
		return nil, err
	}
	priv, x, y, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Key{
		Private: priv,
		Public: &Public{
			TypeId: typeId,
			PublicKey: ecdsa.PublicKey{
				Curve: curve,
				X:     x,
				Y:     y,
			},
		},
	}, nil
}

func NewKeyNil() *Key {
	return &Key{
		Private: nil,
		Public: &Public{
			TypeId: 0,
			PublicKey: ecdsa.PublicKey{
				Curve: nil,
				X:     &big.Int{},
				Y:     &big.Int{},
			},
		},
	}
}

func (this *Public) Equal(other *Public) bool {
	if this.TypeId != other.TypeId {
		return false
	}
	return this.X.Cmp(other.X) == 0 && this.Y.Cmp(other.Y) == 0
}

func (this *Public) Serialize(w io.Writer) error {
	_, err := w.Write(utils.Serialize(this.Serialized()))
	return err
}

func (this *Public) Deserialize(r io.Reader) error {
	var serialized PublicSerialized
	if err := utils.Deserialize(&serialized, r); err != nil {
		return err
	}
	return PublicFromSerialized(this, serialized.TypeId, serialized.X, serialized.Y)
}

func (this *Public) Serialized() PublicSerialized {
	return PublicSerialized{
		TypeId: this.TypeId,
		X:      this.X.Bytes(),
		Y:      this.Y.Bytes(),
	}
}

func (this *Public) SerializedPlain() PublicSerializedPlain {
	return PublicSerializedPlain{
		TypeId: this.TypeId,
		X: &utils.BytesWithoutLengthPrefix{
			Bytes: this.X.Bytes(),
		},
		Y: &utils.BytesWithoutLengthPrefix{
			Bytes: this.Y.Bytes(),
		},
	}
}

func PublicFromSerialized(public *Public, typeID uint16, x []byte, y []byte) error {
	curve, err := CurveById(typeID)
	if err != nil {
		return err
	}

	X := big.NewInt(0).SetBytes(x)
	Y := big.NewInt(0).SetBytes(y)
	if !curve.IsOnCurve(X, Y) {
		return errors.New("Is not on curve")
	}

	public.TypeId = typeID
	public.Curve = curve
	public.X = X
	public.Y = Y
	return nil
}

func NewPublic(data []byte) (*Public, error) {
	var serialized PublicSerialized
	if err := utils.Deserialize(&serialized, bytes.NewBuffer(data)); err != nil {
		return nil, err
	}

	if serialized.TypeId == 0 && len(serialized.X) == 0 && len(serialized.Y) == 0 {
		return NewKeyNil().Public, nil
	}

	var public Public
	if err := PublicFromSerialized(&public, serialized.TypeId, serialized.X, serialized.Y); err != nil {
		return nil, err
	}

	return &public, nil
}
