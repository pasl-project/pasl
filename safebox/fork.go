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

	"github.com/pasl-project/pasl/common"
	"github.com/pasl-project/pasl/defaults"
)

type GetLastTimestamps func(maxCount uint32) []uint32

type Fork interface {
	CheckBlock(currentTarget common.TargetBase, block BlockBase) error
	GetNextTarget(currentTarget common.TargetBase, getLastTimestamps GetLastTimestamps) uint32
	GetBlockHashingBlob(block BlockBase) (template []byte, reservedOffset int, reservedSize int)
}

type ForkActivator interface {
	Activate(prevSafeboxHash []byte) bool
}

type activatorSafebox struct {
	ForkActivator
	prevSafeboxHash [32]byte
}

type ForkInitializer func() Fork

type forkDetails struct {
	activator   ForkActivator
	initializer ForkInitializer
}

var forks = map[uint32]forkDetails{
	0: forkDetails{
		activator: &activatorSafebox{
			prevSafeboxHash: defaults.GenesisSafeBox,
		},
		initializer: func() Fork {
			return &checkpoint{}
		},
	},
	29000: forkDetails{
		activator: &activatorSafebox{
			prevSafeboxHash: [32]byte{0x7A, 0x66, 0xCA, 0x0D, 0x45, 0x03, 0x8E, 0x97, 0xBA, 0xED, 0x24, 0x4B, 0x4B, 0xC5, 0x14, 0x9C, 0x1A, 0x77, 0xE8, 0x83, 0x19, 0x08, 0x20, 0x9F, 0x80, 0xCC, 0x9C, 0x09, 0x89, 0xCE, 0x3A, 0x80},
		},
		initializer: func() Fork {
			return &antiHopDiff{}
		},
	},
}

func GetActiveFork(height uint32, prevSafeboxHash []byte) Fork {
	var initializer ForkInitializer
	var maxHeight uint32
	for activationHeight, details := range forks {
		if height >= activationHeight && height >= maxHeight {
			initializer = details.initializer
			maxHeight = activationHeight
		} else if height < activationHeight && initializer != nil {
			break
		}
	}
	return initializer()
}

func TryActivateFork(height uint32, prevSafeboxHash []byte) Fork {
	if details, ok := forks[height]; ok {
		if details.activator.Activate(prevSafeboxHash) {
			return details.initializer()
		}
	}
	return nil
}

func (activator *activatorSafebox) Activate(prevSafeboxHash []byte) bool {
	return bytes.Equal(prevSafeboxHash, activator.prevSafeboxHash[:])
}
