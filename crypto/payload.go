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
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"

	ecdh "github.com/cs8425/go-ecdh"
	"github.com/fd/eccp"
)

type encryptHeader struct {
	KeyLength    byte
	MAC          byte
	PaddedLength uint16
	Length       uint16
}

func kdf(input []byte) [sha512.Size]byte {
	return sha512.Sum512(input)
}

func mac(key []byte, input []byte) ([]byte, error) {
	mac := hmac.New(md5.New, key)
	if _, err := mac.Write(input); err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

func padToBlockSize(input []byte, blockSize int) []byte {
	size := len(input)
	if size%blockSize == 0 {
		return input
	}
	result := make([]byte, (size+blockSize-1)/blockSize*blockSize)
	copy(result, input)
	return result
}

func aes256Cbc(input []byte, key []byte, iv []byte, encrypt bool) ([]byte, error) {
	c, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}

	if iv == nil {
		iv = make([]byte, c.BlockSize())
	}
	withPadding := padToBlockSize(input, c.BlockSize())
	result := make([]byte, len(withPadding))

	if encrypt {
		cipher.NewCBCEncrypter(c, iv).CryptBlocks(result, withPadding)
	} else {
		cipher.NewCBCDecrypter(c, iv).CryptBlocks(result, withPadding)
	}
	return result, nil
}

func Encrypt(public *ecdsa.PublicKey, payload []byte) ([]byte, error) {
	dh := ecdh.NewEllipticECDH(public.Curve)
	ephimeral, ephimeralPublic, err := dh.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	shared, err := dh.GenerateSharedSecret(ephimeral, public)
	if err != nil {
		return nil, err
	}
	derived := kdf(shared)

	ciphertext, err := aes256Cbc(payload, derived[:32], nil, true)
	if err != nil {
		return nil, err
	}

	MAC, err := mac(derived[32:], ciphertext)
	if err != nil {
		return nil, err
	}

	ephimeralPublicEcdsa := ephimeralPublic.(*ecdsa.PublicKey)
	ephimeralPublicRaw := eccp.Marshal(public.Curve, ephimeralPublicEcdsa.X, ephimeralPublicEcdsa.Y)

	serialized := bytes.NewBuffer(nil)
	binary.Write(serialized, binary.LittleEndian, encryptHeader{
		KeyLength:    byte(len(ephimeralPublicRaw)),
		MAC:          byte(len(MAC)),
		PaddedLength: uint16(len(ciphertext)),
		Length:       uint16(len(payload)),
	})
	if _, err := serialized.Write(ephimeralPublicRaw[:]); err != nil {
		return nil, err
	}
	if _, err := serialized.Write(MAC); err != nil {
		return nil, err
	}
	if _, err := serialized.Write(ciphertext); err != nil {
		return nil, err
	}

	return serialized.Bytes(), nil
}

func Decrypt(private *Key, payload []byte) ([]byte, error) {
	buffer := bytes.NewBuffer(payload)

	header := encryptHeader{}
	if err := binary.Read(buffer, binary.LittleEndian, &header); err != nil {
		return nil, err
	}

	ephimeralPublicRaw := make([]byte, header.KeyLength)
	if _, err := buffer.Read(ephimeralPublicRaw); err != nil {
		return nil, err
	}

	MAC := make([]byte, header.MAC)
	if _, err := buffer.Read(MAC); err != nil {
		return nil, err
	}

	ciphertext := make([]byte, header.PaddedLength)
	if _, err := buffer.Read(ciphertext); err != nil {
		return nil, err
	}

	dh := ecdh.NewEllipticECDH(private.Public.Curve)
	ephimeralPublic := UnmarshalPublic(private.Public.Curve, ephimeralPublicRaw)
	if ephimeralPublic == nil {
		return nil, errors.New("failed to unmarshal ephimeral public key")
	}

	shared, err := dh.GenerateSharedSecret(private.Convert(), ephimeralPublic)
	if err != nil {
		return nil, err
	}
	derived := kdf(shared)

	check, err := mac(derived[32:], ciphertext)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(check, MAC) {
		return nil, fmt.Errorf("invalid MAC")
	}

	plaintext, err := aes256Cbc(ciphertext, derived[:32], nil, false)
	if err != nil {
		return nil, err
	}
	return plaintext[:header.Length], nil
}
