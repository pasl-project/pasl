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
	"github.com/pasl-project/pasl/utils"
)

type packsMap struct {
	max   uint32
	packs map[uint32]*PackBase
}

func newPacksMap() packsMap {
	return packsMap{
		max:   0,
		packs: make(map[uint32]*PackBase, 0),
	}
}

func (p *packsMap) len() int {
	return len(p.packs)
}

func (p *packsMap) keys() []uint32 {
	keys := make([]uint32, 0, len(p.packs))
	for k := range p.packs {
		keys = append(keys, k)
	}
	return keys
}

func (p *packsMap) forEach(fn func(number uint32, pack *PackBase)) {
	for each := range p.packs {
		pack := p.packs[each]
		fn(each, pack)
	}
}

func (p *packsMap) get(number uint32) *PackBase {
	if pack, ok := p.packs[number]; ok {
		return pack
	}
	return nil
}

func (p *packsMap) set(number uint32, pack PackBase) {
	p.packs[number] = &pack
	p.max = utils.MaxUint32(p.max, number)
}

func (p *packsMap) getMax() *uint32 {
	if len(p.packs) == 0 {
		return nil
	}
	result := p.max
	return &result
}
