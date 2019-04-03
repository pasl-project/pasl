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

package safebox

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/pasl-project/pasl/utils"
)

func TestMetaSerialize(t *testing.T) {
	valid, _ := hex.DecodeString("0201000100000000004600ca02200059a6ef47d508cdd935d9841dc377555697b414c7a9daaa9ba289f9cee6fedd3220004ba82df4966794b2b33e1db8f8d7e18bc0d401012db9a169d22eaaa321cad41e20a107000000000000000000000000009f2f92580000002470a2f7322a004e6577204e6f646520322f312f323031372031313a35363a3333202d20204275696c643a742f312d2d2d2000dc9388917fb00065999f25bde135617677c7020a3aea916098b39ede89e37a222000e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b8552000000000000eae7a91b748c735a5338a11715d815101e0c075f7c60fa52b769ec700000000")
	var it SerializedBlock
	if err := utils.Deserialize(&it, bytes.NewBuffer(valid)); err != nil {
		t.FailNow()
	}
	meta := BlockMetadata{
		Index:           it.Header.Index,
		Miner:           it.Header.Miner,
		Version:         it.Header.Version,
		Timestamp:       it.Header.Time,
		Target:          it.Header.Target,
		Nonce:           it.Header.Nonce,
		Payload:         it.Header.Payload,
		PrevSafeBoxHash: it.Header.PrevSafeboxHash,
		Operations:      it.Operations,
	}

	serialized := utils.Serialize(&meta)
	var check BlockMetadata
	if err := utils.Deserialize(&check, bytes.NewBuffer(serialized)); err != nil {
		t.FailNow()
	}

	if !bytes.Equal(serialized, utils.Serialize(check)) {
		t.FailNow()
	}
}

func newBlock(payloadLength int) error {
	_, err := NewBlock(&BlockMetadata{
		Payload: make([]byte, payloadLength),
	})
	return err
}

func TestInvalidBlock(t *testing.T) {
	if err := newBlock(255); err != nil {
		t.FailNow()
	}
	if err := newBlock(256); err == nil {
		t.FailNow()
	}
}
