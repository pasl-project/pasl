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
	"errors"
	"fmt"
	"io"

	"github.com/pasl-project/pasl/accounter"
	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"
)

type ChangeKey struct {
	Source       uint32
	OperationId  uint32
	Fee          uint64
	Payload      []byte
	PublicKey    crypto.Public
	NewPublickey []byte
	Signature    crypto.SignatureSerialized
}

type changeKeyContext struct {
	NewPublic *crypto.Public
}

type changeKeyToSign struct {
	Source    uint32
	Operation uint32
	Fee       uint64
	Payload   utils.Serializable
	Public    crypto.PublicSerializedPlain
	NewPublic utils.Serializable
}

func (this *ChangeKey) GetAccount() uint32 {
	return this.Source
}

func (this *ChangeKey) GetAmount() uint64 {
	return 0
}

func (this *ChangeKey) GetDestAccount() uint32 {
	return this.Source
}

func (this *ChangeKey) GetFee() uint64 {
	return this.Fee
}

func (this *ChangeKey) GetPayload() []byte {
	return this.Payload
}

func (this *ChangeKey) validate(getAccount func(number uint32) *accounter.Account) (context interface{}, err error) {
	source := getAccount(this.Source)
	if source == nil {
		return nil, fmt.Errorf("Source account %d not found", this.Source)
	}
	if source.GetOperationsCount()+1 != this.OperationId {
		return nil, fmt.Errorf("Invalid source account %d operation index %d != %d expected", source.GetNumber(), this.OperationId, source.GetOperationsCount()+1)
	}
	if source.GetBalance() < this.Fee {
		return nil, errors.New("Insufficient balance")
	}

	public, err := crypto.NewPublic(this.NewPublickey)
	if err != nil {
		return nil, err
	}

	// TODO: temporarily disabled, there are some blocks containing such txes
	// if public.Equal(source.GetPublicKey()) {
	// 	return nil, fmt.Errorf("New public key is equal to the current one")
	// }

	return &changeKeyContext{public}, nil
}

func (this *ChangeKey) Apply(index uint32, context interface{}, accounter *accounter.Accounter) ([]uint32, error) {
	params := context.(*changeKeyContext)
	accounter.KeyChange(this.Source, params.NewPublic, index, this.Fee)

	return []uint32{this.Source}, nil
}

func (this *ChangeKey) SerializeWithoutPrefix(w io.Writer) error {
	_, err := w.Write(utils.Serialize(this))
	return err
}

func (this *ChangeKey) getBufferToSign() []byte {
	return utils.Serialize(changeKeyToSign{
		Source:    this.Source,
		Operation: this.OperationId,
		Fee:       this.Fee,
		Payload: &utils.BytesWithoutLengthPrefix{
			Bytes: this.Payload,
		},
		Public: this.PublicKey.SerializedPlain(),
		NewPublic: &utils.BytesWithoutLengthPrefix{
			Bytes: this.NewPublickey,
		},
	})
}

func (this *ChangeKey) getSignature() *crypto.SignatureSerialized {
	return &this.Signature
}

func (this *ChangeKey) getSourceInfo() (number uint32, operationId uint32, publicKey *crypto.Public) {
	return this.Source, this.OperationId, &this.PublicKey
}

func (this *ChangeKey) GetType() txType {
	return txTypeChangekey
}
