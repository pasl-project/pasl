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
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

type commonOperation interface {
	GetAmount() uint64
	GetAccount() uint32
	GetDestAccount() uint32
	GetFee() uint64
	GetPayload() []byte

	Validate(getAccount func(number uint32) *accounter.Account) (context interface{}, err error)
	Apply(index uint32, context interface{}) (map[uint32][]accounter.Micro, error)

	Serialize(w io.Writer) error

	getBufferToSign() []byte
	getSignature() *crypto.SignatureSerialized
	getSourceInfo() (number uint32, operationId uint32, publicKey *crypto.Public)
}

// TODO: rename to transaction
type Tx struct {
	Type txType
	commonOperation
}

type OperationsNetwork struct {
	Operations []Tx
}

func (this *Tx) GetFee() uint64 {
	return this.commonOperation.GetFee()
}

func (this *Tx) Validate(getAccount func(number uint32) *accounter.Account) (context interface{}, err error) {
	number, _, publicKey := this.commonOperation.getSourceInfo()

	source := getAccount(number)
	if source == nil {
		return nil, fmt.Errorf("Source account %d not found", number)
	}
	if !source.PublicKey.Equal(publicKey) {
		return nil, errors.New("Source account invalid public key")
	}

	if err := checkSignature(publicKey, this.commonOperation.getBufferToSign(), this.commonOperation.getSignature()); err != nil {
		return nil, err
	}

	return this.commonOperation.Validate(getAccount)
}

func (this *Tx) GetTxId() []byte {
	type toHash struct {
		ToSign utils.Serializable
		R      utils.Serializable
		S      utils.Serializable
	}
	buffer := utils.Serialize(toHash{
		ToSign: &utils.BytesWithoutLengthPrefix{
			Bytes: this.commonOperation.getBufferToSign(),
		},
		R: &utils.BytesWithoutLengthPrefix{
			Bytes: this.getSignature().R,
		},
		S: &utils.BytesWithoutLengthPrefix{
			Bytes: this.getSignature().S,
		},
	})

	hash := ripemd160.New()
	if _, err := hash.Write(buffer); err != nil {
		return nil
	}

	type txId struct {
		Reserved    uint32
		Source      uint32
		OperationId uint32
		Hash        utils.Serializable
	}
	source, operationId, _ := this.commonOperation.getSourceInfo()

	return utils.Serialize(txId{
		Reserved:    0,
		Source:      source,
		OperationId: operationId,
		Hash: &utils.BytesWithoutLengthPrefix{
			Bytes: []byte(strings.ToUpper(hex.EncodeToString(hash.Sum([]byte("")))[:20])),
		},
	})
}

func (this *Tx) GetTxIdString() string {
	return hex.EncodeToString(this.GetTxId())
}

func checkSignature(public *crypto.Public, data []byte, signatureSerialized *crypto.SignatureSerialized) error {
	signature := signatureSerialized.Decompress()
	if !ecdsa.Verify(&public.PublicKey, data, signature.R, signature.S) {
		return errors.New("Invalid signature")
	}
	return nil
}

func (this *Tx) Serialize(w io.Writer) error {
	if _, err := w.Write(utils.Serialize(&this.Type)); err != nil {
		return err
	}
	return this.SerializeUnderlying(w)
}

func (this *Tx) SerializeUnderlying(w io.Writer) error {
	return this.commonOperation.Serialize(w)
}

func (this *Tx) deserializeUnderlying(r io.Reader) error {
	switch this.Type {
	case txTypeTransfer:
		var tx Transfer
		if err := utils.Deserialize(&tx, r); err != nil {
			return err
		}
		this.commonOperation = &tx
		return nil
	case txTypeChangekey:
		var changeKey ChangeKey
		if err := utils.Deserialize(&changeKey, r); err != nil {
			return err
		}
		this.commonOperation = &changeKey
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
		if _, err := w.Write(utils.Serialize(uint8(each.Type))); err != nil {
			return err
		}
		if err := each.SerializeUnderlying(w); err != nil {
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

	this.Operations = make([]Tx, count)

	var i uint32
	for i = 0; i < count; i++ {
		var transactionType uint8
		if err := utils.Deserialize(&transactionType, r); err != nil {
			return err
		}
		this.Operations[i].Type = txType(transactionType)
		if err := this.Operations[i].deserializeUnderlying(r); err != nil {
			return err
		}
	}

	return nil
}
