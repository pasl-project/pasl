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

type Transfer struct {
	Source      uint32
	OperationId uint32
	Destination uint32
	Amount      uint64
	Fee         uint64
	Payload     []byte
	PublicKey   crypto.Public
	Signature   crypto.SignatureSerialized
}

type transferContext struct {
	Source      *accounter.Account
	Destination *accounter.Account
}

type transferToSign struct {
	Source      uint32
	Operation   uint32
	Destination uint32
	Amount      uint64
	Fee         uint64
	Payload     utils.Serializable
	Public      crypto.PublicSerializedPlain
}

func (this *Transfer) GetAccount() uint32 {
	return this.Source
}

func (this *Transfer) GetAmount() uint64 {
	return this.Amount
}

func (this *Transfer) GetDestAccount() uint32 {
	return this.Destination
}

func (this *Transfer) GetFee() uint64 {
	return this.Fee
}

func (this *Transfer) GetPayload() []byte {
	return this.Payload
}

func (this *Transfer) Validate(getAccount func(number uint32) *accounter.Account) (context interface{}, err error) {
	destination := getAccount(this.Destination)
	if destination == nil {
		return nil, fmt.Errorf("Destination account %d not found", this.Destination)
	}

	source := getAccount(this.Source)
	if source == nil {
		return nil, fmt.Errorf("Source account %d not found", this.Source)
	}
	if source.Operations+1 != this.OperationId {
		return nil, fmt.Errorf("Invalid source account %d operation index %d != %d expected", source.Number, this.OperationId, source.Operations+1)
	}
	if source.Balance < this.Amount {
		return nil, errors.New("Insufficient balance")
	}
	if source.Balance-this.Amount < this.Fee {
		return nil, errors.New("Insufficient balance")
	}
	if 0xFFFFFFFFFFFFFFFF-destination.Balance < this.Amount {
		return nil, errors.New("Uint64 overflow")
	}

	return &transferContext{source, destination}, nil
}

func (this *Transfer) Apply(index uint32, context interface{}) (map[uint32][]accounter.Micro, error) {
	params := context.(*transferContext)

	result := make(map[uint32][]accounter.Micro)
	result[params.Source.Number] = params.Source.BalanceSub(this.Amount+this.Fee, index)
	result[params.Destination.Number] = params.Destination.BalanceAdd(this.Amount, index)
	return result, nil
}

func (this *Transfer) Serialize(w io.Writer) error {
	_, err := w.Write(utils.Serialize(this))
	return err
}

func (this *Transfer) getBufferToSign() []byte {
	return utils.Serialize(transferToSign{
		Source:      this.Source,
		Operation:   this.OperationId,
		Destination: this.Destination,
		Amount:      this.Amount,
		Fee:         this.Fee,
		Payload: &utils.BytesWithoutLengthPrefix{
			Bytes: this.Payload,
		},
		Public: this.PublicKey.SerializedPlain(),
	})
}

func (this *Transfer) getSourceInfo() (number uint32, operationId uint32, publicKey *crypto.Public) {
	return this.Source, this.OperationId, &this.PublicKey
}

func (this *Transfer) getSignature() *crypto.SignatureSerialized {
	return &this.Signature
}
