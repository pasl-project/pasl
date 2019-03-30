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

package accounter

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/pasl-project/pasl/crypto"
)

func TestSerialize(t *testing.T) {
	key, err := crypto.NewKeyByType(crypto.NIDsecp256k1)
	if err != nil {
		t.FailNow()
	}

	accounter := NewAccounter()
	accounter.NewPack(key.Public, 1, 2, big.NewInt(3))

	buffer, err := accounter.Marshal()
	if err != nil {
		t.FailNow()
	}

	other := NewAccounter()
	if _, err = other.Unmarshal(buffer); err != nil {
		t.FailNow()
	}

	_, hash, _ := accounter.GetState()
	_, hash2, _ := other.GetState()
	if !bytes.Equal(hash, hash2) {
		t.FailNow()
	}
}
