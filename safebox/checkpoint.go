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

package safebox

import (
	"github.com/pasl-project/pasl/common"
)

type checkpoint struct {
	Fork
}

func (this *checkpoint) CheckBlock(currentTarget common.TargetBase, block BlockBase) error {
	return nil
}

func (this *checkpoint) GetNextTarget(currentTarget common.TargetBase, getLastTimestamps GetLastTimestamps) uint32 {
	return currentTarget.GetCompact()
}
