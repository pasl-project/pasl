/*
PASL - Personalized Accounts & Secure Ledger

Copyright (C) 2018 PASL Project

Greatly inspired by Kurt Rose's python implementation
https://gist.github.com/kurtbrose/4423605

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
	"encoding/hex"
	"testing"
)

func TestFromCompact(t *testing.T) {
	valid := "e5d4c38000000000000000000000000000000000000000000000000"
	if got := fromCompact(0x12345678).Text(16); got != valid {
		t.Errorf("\n%s !=\n%s", got, valid)
	}

	valid = "ffffff8000000000000000000000000000000000000000000000000"
	if got := fromCompact(0x24000000).Text(16); got != valid {
		t.Errorf("\n%s !=\n%s", got, valid)
	}

	valid = "2f84f800000000000000000000000000000000000000000000000"
	if got := fromCompact(0x2E83D83F).Text(16); got != valid {
		t.Errorf("\n%s !=\n%s", got, valid)
	}
}

func TestCheck(t *testing.T) {
	target := NewTarget(0x12345678)
	check, _ := hex.DecodeString("0e5d4c38000000000000000000000000000000000000000000000000")
	if !target.Check(check) {
		t.FailNow()
	}
	check, _ = hex.DecodeString("0e5d4c38000000000000000000000000000000000000000000000001")
	if target.Check(check) {
		t.FailNow()
	}
	check, _ = hex.DecodeString("0e5d4c37000000000000000000000000000000000000000000000000")
	if !target.Check(check) {
		t.FailNow()
	}
}
