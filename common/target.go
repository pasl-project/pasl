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

package common

import (
	"encoding/binary"
	"io"
	"math/big"

	"github.com/pasl-project/pasl/defaults"
	"github.com/pasl-project/pasl/utils"
)

var bigOne = big.NewInt(1)
var difficultyOne *big.Int = big.NewInt(0).Lsh(bigOne, 256)

type target struct {
	compact uint32
	value   *big.Int
}

type TargetBase interface {
	GetCompact() uint32
	Get() *big.Int
	GetDifficulty() *big.Int
	Check(pow []byte) bool
	Equal(other TargetBase) bool

	utils.Serializable
}

func NewTarget(compact uint32) TargetBase {
	value := fromCompact(compact)
	return &target{
		compact: ToCompact(value),
		value:   value,
	}
}

func (this *target) GetCompact() uint32 {
	return this.compact
}

func (this *target) Get() *big.Int {
	return this.value
}

func (this *target) GetDifficulty() *big.Int {
	return big.NewInt(0).Div(difficultyOne, this.Get())
}

func (this *target) Check(pow []byte) bool {
	result := &big.Int{}
	result.SetBytes(pow)
	return result.Cmp(this.value) < 1
}

func (this *target) Equal(other TargetBase) bool {
	return this.compact == other.GetCompact()
}

func (this *target) Serialize(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, this.compact)
}

func (this *target) Deserialize(r io.Reader) error {
	compact := uint32(0)
	err := binary.Read(r, binary.LittleEndian, &compact)
	if err == nil {
		this.value = fromCompact(compact)
		this.compact = ToCompact(this.value)
	}
	return err
}

func fromCompact(compact uint32) *big.Int {
	value := (compact&0x00FFFFFF ^ 0x00FFFFFF) | 0x01000000
	zeroBits := uint(compact >> 24)
	if zeroBits < defaults.MinTargetBits {
		zeroBits = defaults.MinTargetBits
	} else if zeroBits > 231 {
		zeroBits = 231
	}

	result := big.NewInt(int64(value))
	return result.Lsh(result, 256-zeroBits-25)
}

func fromDifficulty(difficulty uint64) uint32 {
	return ToCompact(big.NewInt(0).Div(difficultyOne, big.NewInt(0).SetUint64(difficulty)))
}

func ToCompact(value *big.Int) uint32 {
	tmp := big.NewInt(0).Set(value)
	a := big.NewInt(0xFFFFFF)
	bits := tmp.BitLen()
	tmp.Rsh(tmp, uint(bits-25)).Xor(tmp, a).And(tmp, a)
	return uint32(256-bits)<<24 | uint32(tmp.Uint64())
}
