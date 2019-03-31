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

package tx

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/pasl-project/pasl/accounter"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"

	"golang.org/x/crypto/ripemd160"
)

type txType uint32

const (
	_ txType = iota
	txTypeTransfer
	txTypeChangekey
)

type CommonOperation interface {
	GetAmount() uint64
	GetAccount() uint32
	GetDestAccount() uint32
	GetFee() uint64
	GetPayload() []byte
	GetType() txType

	Apply(index uint32, context interface{}, accounter *accounter.Accounter) ([]uint32, error)

	Serialize(w io.Writer) error
	SerializeWithoutPrefix(w io.Writer) error

	getBufferToSign() []byte
	getSignature() *crypto.SignatureSerialized
	getSourceInfo() (number uint32, operationId uint32, publicKey *crypto.Public)
	validate(getAccount func(number uint32) *accounter.Account) (context interface{}, err error)
}

// TODO: rename to transaction
type Tx struct {
	Type txType
	CommonOperation
}

type TxMetadata struct {
	BlockIndex uint32
	Index      uint32
	Time       uint32
	Type       uint8
	TxRaw      []byte
}

type OperationsNetwork struct {
	Operations []CommonOperation
}

func Sign(tx CommonOperation, priv *ecdsa.PrivateKey) (txID string, raw []byte, err error) {
	_, _, public := tx.getSourceInfo()
	public.Curve = priv.Curve
	public.X = big.NewInt(0).Set(priv.PublicKey.X)
	public.Y = big.NewInt(0).Set(priv.PublicKey.Y)

	data := tx.getBufferToSign()
	r, s, err := ecdsa.Sign(rand.Reader, priv, data)
	if err != nil {
		return "", nil, err
	}

	signature := tx.getSignature()
	signature.R = r.Bytes()
	signature.S = s.Bytes()

	operations := OperationsNetwork{
		Operations: []CommonOperation{tx},
	}
	serialized := bytes.NewBuffer(nil)
	if err := operations.Serialize(serialized); err != nil {
		return "", nil, err
	}
	return GetTxIdString(tx), serialized.Bytes(), nil
}

func ValidateSignature(tx CommonOperation) error {
	_, _, publicKey := tx.getSourceInfo()
	return checkSignature(publicKey, tx.getBufferToSign(), tx.getSignature())
}

func Validate(tx CommonOperation, getAccount func(number uint32) *accounter.Account) (context interface{}, err error) {
	number, _, publicKey := tx.getSourceInfo()

	source := getAccount(number)
	if source == nil {
		return nil, fmt.Errorf("Source account %d not found", number)
	}
	if !source.IsPublicKeyEqual(publicKey) {
		return nil, errors.New("Source account invalid public key")
	}

	return tx.validate(getAccount)
}

func GetRipemd16Hash(tx CommonOperation) []byte {
	type toHash struct {
		ToSign utils.Serializable
		R      utils.Serializable
		S      utils.Serializable
	}
	buffer := utils.Serialize(toHash{
		ToSign: &utils.BytesWithoutLengthPrefix{
			Bytes: tx.getBufferToSign(),
		},
		R: &utils.BytesWithoutLengthPrefix{
			Bytes: tx.getSignature().R,
		},
		S: &utils.BytesWithoutLengthPrefix{
			Bytes: tx.getSignature().S,
		},
	})

	hash := ripemd160.New()
	if _, err := hash.Write(buffer); err != nil {
		utils.Panicf("Failed to produce Ripemd-160 hash")
	}

	return []byte(strings.ToUpper(hex.EncodeToString(hash.Sum([]byte("")))[:20]))
}

func GetTxId(tx CommonOperation) []byte {
	type txId struct {
		Reserved    uint32
		Source      uint32
		OperationId uint32
		Hash        utils.Serializable
	}
	source, operationId, _ := tx.getSourceInfo()

	return utils.Serialize(txId{
		Reserved:    0,
		Source:      source,
		OperationId: operationId,
		Hash: &utils.BytesWithoutLengthPrefix{
			Bytes: GetRipemd16Hash(tx),
		},
	})
}

func GetTxIdString(tx CommonOperation) string {
	return hex.EncodeToString(GetTxId(tx))
}

func checkSignature(public *crypto.Public, data []byte, signatureSerialized *crypto.SignatureSerialized) error {
	signature := signatureSerialized.Decompress()
	if !ecdsa.Verify(&public.PublicKey, data, signature.R, signature.S) {
		return errors.New("Invalid signature")
	}
	return nil
}

func GetMetadata(tx CommonOperation, txIndexInsideBlock uint32, blockIndex uint32, time uint32) TxMetadata {
	return TxMetadata{
		BlockIndex: blockIndex,
		Index:      txIndexInsideBlock,
		Time:       time,
		Type:       uint8(tx.GetType()),
		TxRaw:      utils.Serialize(tx),
	}
}

func TxFromMetadata(metadataSerialized []byte) (*TxMetadata, *Tx, error) {
	var metadata TxMetadata
	if err := utils.Deserialize(&metadata, bytes.NewBuffer(metadataSerialized)); err != nil {
		return nil, nil, err
	}

	var tx Tx
	if err := tx.Deserialize(bytes.NewBuffer(metadata.TxRaw)); err != nil {
		return nil, nil, fmt.Errorf("Failed to deserialize tx: %v", err)
	}

	return &metadata, &tx, nil
}

func (this *Tx) SerializeUnderlying(w io.Writer) error {
	return this.CommonOperation.Serialize(w)
}

func (this *Tx) deserializeUnderlying(r io.Reader) error {
	switch this.Type {
	case txTypeTransfer:
		var tx Transfer
		if err := utils.Deserialize(&tx, r); err != nil {
			return err
		}
		this.CommonOperation = &tx
		return nil
	case txTypeChangekey:
		var changeKey ChangeKey
		if err := utils.Deserialize(&changeKey, r); err != nil {
			return err
		}
		this.CommonOperation = &changeKey
		return nil
	default:
		return errors.New("Unknown operation type")
	}
}

func (this *Tx) Deserialize(r io.Reader) error {
	if err := utils.Deserialize(&this.Type, r); err != nil {
		return err
	}
	return this.deserializeUnderlying(r)
}

func (this *OperationsNetwork) Serialize(w io.Writer) error {
	if _, err := w.Write(utils.Serialize(uint32(len(this.Operations)))); err != nil {
		return err
	}

	for _, each := range this.Operations {
		if _, err := w.Write(utils.Serialize(uint8(each.GetType()))); err != nil {
			return err
		}
		if err := each.SerializeWithoutPrefix(w); err != nil {
			return err
		}
	}

	return nil
}

func (this *OperationsNetwork) Deserialize(r io.Reader) error {
	var count uint32
	if err := utils.Deserialize(&count, r); err != nil {
		return err
	}

	this.Operations = make([]CommonOperation, count)

	var i uint32
	for i = 0; i < count; i++ {
		var transactionType uint8
		if err := utils.Deserialize(&transactionType, r); err != nil {
			return err
		}

		switch txType(transactionType) {
		case txTypeTransfer:
			var tx Transfer
			if err := utils.Deserialize(&tx, r); err != nil {
				return err
			}
			this.Operations[i] = &tx
			return nil
		case txTypeChangekey:
			var changeKey ChangeKey
			if err := utils.Deserialize(&changeKey, r); err != nil {
				return err
			}
			this.Operations[i] = &changeKey
			return nil
		default:
			return errors.New("Unknown operation type")
		}
	}

	return nil
}
