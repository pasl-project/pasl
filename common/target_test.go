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
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
)

func TestFromCompact(t *testing.T) {
	valid, _ := big.NewInt(0).SetString("000000000e5d4c38000000000000000000000000000000000000000000000000", 16)
	if got := NewTarget(0x12345678).Get(); got.Cmp(valid) != 0 {
		t.Errorf("\n%s !=\n%s", got, valid)
	}

	target, _ := big.NewInt(0).SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
	if NewTarget(ToCompact(target)).GetDifficulty().Uint64() != 68719478784 {
		t.FailNow()
	}
	target, _ = big.NewInt(0).SetString("000000000ffffff8000000000000000000000000000000000000000000000000", 16)
	if NewTarget(ToCompact(target)).GetDifficulty().Uint64() != 68719478784 {
		t.FailNow()
	}
	target, _ = big.NewInt(0).SetString("000000000ffffff0000000000000000000000000000000000000000000000000", 16)
	difficulty := uint64(68719480832)
	targeBase := NewTarget(ToCompact(target))
	if targeBase.GetDifficulty().Uint64() != difficulty {
		t.FailNow()
	}
	if !NewTarget(fromDifficulty(difficulty)).Equal(targeBase) {
		t.FailNow()
	}

	valid, _ = big.NewInt(0).SetString("000000000ffffff8000000000000000000000000000000000000000000000000", 16)
	if got := NewTarget(0x00000000).Get(); got.Cmp(valid) != 0 {
		t.Errorf("\n%s !=\n%s", got, valid)
	}
	if got := NewTarget(0x24000000).Get(); got.Cmp(valid) != 0 {
		t.Errorf("\n%s !=\n%s", got, valid)
	}

	valid, _ = big.NewInt(0).SetString("000000000002f84f800000000000000000000000000000000000000000000000", 16)
	if got := NewTarget(0x2E83D83F).Get(); got.Cmp(valid) != 0 {
		t.Errorf("\n%s !=\n%s", got, valid)
	}

	if NewTarget(0x00000000).GetCompact() != 0x24000000 {
		t.FailNow()
	}
	if NewTarget(0xFF000000).GetCompact() != 0xE7000000 {
		t.FailNow()
	}
}

func TestCheck(t *testing.T) {
	targetInt, _ := big.NewInt(0).SetString("000000000e5d4c38000000000000000000000000000000000000000000000000", 16)
	target := NewTarget(ToCompact(targetInt))
	check, _ := hex.DecodeString("000000000e5d4c38000000000000000000000000000000000000000000000000")
	if !target.Check(check) {
		t.FailNow()
	}
	check, _ = hex.DecodeString("000000000e5d4c38000000000000000000000000000000000000000000000001")
	if target.Check(check) {
		t.FailNow()
	}
	check, _ = hex.DecodeString("000000000e5d4c37ffffffffffffffffffffffffffffffffffffffffffffffff")
	if !target.Check(check) {
		t.FailNow()
	}
}

func Test_fromDifficulty(t *testing.T) {
	type args struct {
		difficulty uint64
	}
	tests := []struct {
		name string
		args args
		want uint32
	}{
		{"", args{68719478784}, 0x24000000},
		{"", args{68719480832}, 0x24000001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromDifficulty(tt.args.difficulty); got != tt.want {
				t.Errorf("fromDifficulty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSerializeDeserialize(t *testing.T) {
	buffer := bytes.NewBuffer(nil)

	target := NewTarget(0x12345678)
	if err := target.Serialize(buffer); err != nil {
		t.Fatal(err)
	}

	deserialized := NewTarget(0)
	if err := deserialized.Deserialize(buffer); err != nil {
		t.Fatal(err)
	}

	if !target.Equal(deserialized) {
		t.FailNow()
	}
	if target.Get().Cmp(deserialized.Get()) != 0 {
		t.FailNow()
	}
}
