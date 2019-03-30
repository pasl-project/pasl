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
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/pasl-project/pasl/utils"

	"github.com/akamensky/base58"
	"github.com/coin-network/curve"
	"github.com/fd/eccp"
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

type PublicPrefixChecksum struct {
	Prefix   uint8
	Public   PublicSerialized
	Checksum uint32
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

func NewKeyFromPrivate(typeID uint16, private []byte) (*Key, error) {
	curve, err := CurveById(typeID)
	if err != nil {
		return nil, err
	}
	x, y := curve.ScalarBaseMult(private)
	return NewKey(private, typeID, curve, x, y), nil
}

func NewKeyByType(typeID uint16) (*Key, error) {
	curve, err := CurveById(typeID)
	if err != nil {
		return nil, err
	}
	priv, _, _, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}
	return NewKeyFromPrivate(typeID, priv)
}

func NewKey(private []byte, typeID uint16, curve elliptic.Curve, x *big.Int, y *big.Int) *Key {
	return &Key{
		Private: private,
		Public: &Public{
			TypeId: typeID,
			PublicKey: ecdsa.PublicKey{
				Curve: curve,
				X:     x,
				Y:     y,
			},
		},
	}
}

func (k *Key) Convert() *ecdsa.PrivateKey {
	return &ecdsa.PrivateKey{
		D: big.NewInt(0).SetBytes(k.Private),
		PublicKey: ecdsa.PublicKey{
			Curve: k.Public.Curve,
			X:     big.NewInt(0).Set(k.Public.X),
			Y:     big.NewInt(0).Set(k.Public.Y),
		},
	}
}

func (this *Public) Equal(other *Public) bool {
	if this.TypeId != other.TypeId {
		return false
	}
	return this.X.Cmp(other.X) == 0 && this.Y.Cmp(other.Y) == 0
}

func PublicFromBase58(public string) (*Public, error) {
	decoded, err := base58.Decode(public)
	if err != nil {
		return nil, err
	}

	data := PublicPrefixChecksum{}
	if err := utils.Deserialize(&data, bytes.NewBuffer(decoded)); err != nil {
		return nil, err
	}

	if data.Prefix != 0x01 {
		return nil, fmt.Errorf("invalid prefx")
	}

	key := &Public{}
	if err := PublicFromSerialized(key, data.Public.TypeId, data.Public.X, data.Public.Y); err != nil {
		return nil, err
	}
	if key.Checksum() != data.Checksum {
		return nil, fmt.Errorf("invalid checksum")
	}

	return key, nil
}

func (p *Public) ToBase58() string {
	return base58.Encode(utils.Serialize(&PublicPrefixChecksum{
		Prefix:   0x01,
		Public:   p.Serialized(),
		Checksum: p.Checksum(),
	}))
}

func (p *Public) Checksum() uint32 {
	serialized := p.Serialized()
	toChecksum := utils.Serialize(&serialized)
	hash := sha256.Sum256(toChecksum)
	return binary.LittleEndian.Uint32(hash[:4])
}

func (p *Public) Convert() *ecdsa.PublicKey {
	return &ecdsa.PublicKey{
		Curve: p.Curve,
		X:     big.NewInt(0).Set(p.X),
		Y:     big.NewInt(0).Set(p.Y),
	}
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
		return NewKey(nil, 0, nil, big.NewInt(0), big.NewInt(0)).Public, nil
	}

	var public Public
	if err := PublicFromSerialized(&public, serialized.TypeId, serialized.X, serialized.Y); err != nil {
		return nil, err
	}

	return &public, nil
}

func (p *Public) Copy() Public {
	return Public{
		TypeId: p.TypeId,
		PublicKey: ecdsa.PublicKey{
			Curve: p.PublicKey.Curve,
			X:     big.NewInt(0).Set(p.PublicKey.X),
			Y:     big.NewInt(0).Set(p.PublicKey.Y),
		},
	}
}

func unmarshal(c elliptic.Curve, data []byte) (x, y *big.Int) {
	switch c {
	case curve.S256():
		{
			// Copyright Simon Menke "fd" https://github.com/fd
			// https://github.com/fd/eccp/blob/master/eccp.go
			byteLen := (c.Params().BitSize + 7) >> 3

			if len(data) != 1+byteLen {
				// wrong length; fallback to uncompressed
				return elliptic.Unmarshal(c, data)
			}

			if data[0] != 0x02 && data[0] != 0x03 {
				// wrong header; fallback to uncompressed
				return elliptic.Unmarshal(c, data)
			}

			x = new(big.Int).SetBytes(data[1 : 1+byteLen])

			y = new(big.Int)

			/* y = x^2 */
			y.Mul(x, x)
			y.Mod(y, c.Params().P)

			/* y = x^3 */
			y.Mul(y, x)
			y.Mod(y, c.Params().P)

			/* y = x^3 + b */
			y.Add(y, c.Params().B)
			y.Mod(y, c.Params().P)

			modSqrt(y, c, y)

			if y.Bit(0) != uint(data[0]&0x01) {
				y.Sub(c.Params().P, y)
			}

			return x, y
		}
	case Sect283k1():
		{
			return nil, nil
		}
	default:
		return eccp.Unmarshal(c, data)
	}
}

func UnmarshalPublic(c elliptic.Curve, data []byte) *ecdsa.PublicKey {
	x, y := unmarshal(c, data)
	if x == nil || y == nil {
		return nil
	}
	if !c.IsOnCurve(x, y) {
		return nil
	}
	return &ecdsa.PublicKey{
		Curve: c,
		X:     x,
		Y:     y,
	}
}

// Copyright Simon Menke "fd" https://github.com/fd
// https://github.com/fd/eccp/blob/master/eccp.go
func modSqrt(z *big.Int, curve elliptic.Curve, a *big.Int) *big.Int {
	p1 := big.NewInt(1)
	p1.Add(p1, curve.Params().P)

	result := big.NewInt(1)

	for i := p1.BitLen() - 1; i > 1; i-- {
		result.Mul(result, result)
		result.Mod(result, curve.Params().P)
		if p1.Bit(i) > 0 {
			result.Mul(result, a)
			result.Mod(result, curve.Params().P)
		}
	}

	z.Set(result)
	return z
}
