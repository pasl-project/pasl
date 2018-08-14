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
	"testing"
)

func TestReward(t *testing.T) {
	if getReward(0) != 500000 {
		t.FailNow()
	}

	if getReward(420479) != 500000 {
		t.FailNow()
	}

	if getReward(420480) != 250000 {
		t.FailNow()
	}

	if getReward(1000000000) != 10000 {
		t.FailNow()
	}
}
