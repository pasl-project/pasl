/*
Basic Elliptic Curve primitives over Binary Field GF(2ⁿ)

Copyright (C) 2018 Xiphon

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

package ellipticbinary

import (
	"math/big"
)

type binaryFieldInt struct {
	value *big.Int
}

func newBianryFieldInt(value *big.Int) *binaryFieldInt {
	return &binaryFieldInt{
		value: big.NewInt(0).Set(value),
	}
}

func (z *binaryFieldInt) add(x, y *binaryFieldInt) *binaryFieldInt {
	z.value.Xor(x.value, y.value)
	return z
}

func (z *binaryFieldInt) mul(x, y *binaryFieldInt) *binaryFieldInt {
	acc := big.NewInt(0)
	shift := uint(0)
	zero := big.NewInt(0)

	o := big.NewInt(0).Set(y.value)
	tmp := big.NewInt(0)

	for ; o.Cmp(zero) != 0; shift++ {
		if o.Bit(0) != 0 {
			tmp.Lsh(x.value, shift)
			acc.Xor(acc, tmp)
		}
		o.Rsh(o, 1)
	}

	z.value = acc
	return z
}

func (z *binaryFieldInt) mod(x, y *binaryFieldInt) *binaryFieldInt {
	_, z.value = bfDiv(x, y)
	return z
}

func (z *binaryFieldInt) div(x, y *binaryFieldInt) *binaryFieldInt {
	z.value, _ = bfDiv(x, y)
	return z
}

func bfDiv(x, y *binaryFieldInt) (*big.Int, *big.Int) {
	r := big.NewInt(0).Set(x.value)
	q := big.NewInt(0)

	rlen := r.BitLen()
	blen := y.value.BitLen()

	sweeper := big.NewInt(1)
	sweeper.Lsh(sweeper, uint(rlen)-1)

	tmp := big.NewInt(0)
	one := big.NewInt(1)
	zero := big.NewInt(0)
	for rlen >= blen {
		shift := uint(rlen - blen)
		q.Or(q, tmp.Lsh(one, shift))
		r.Xor(r, tmp.Lsh(y.value, shift))
		if r.Cmp(zero) == 0 {
			break
		}
		for r.Cmp(zero) != 0 {
			if tmp.And(sweeper, r).Cmp(zero) != 0 {
				break
			}
			sweeper.Rsh(sweeper, 1)
			rlen--
		}
	}

	return q, r
}

func (z *binaryFieldInt) cmp(x *binaryFieldInt) int {
	return z.value.Cmp(x.value)
}

func (z *binaryFieldInt) divmod(num, den, p *binaryFieldInt) *binaryFieldInt {
	inv, _ := extendedGCD(den, p)
	z.value = inv.mul(inv, num).value
	return z
}

func extendedGCD(ina, inb *binaryFieldInt) (*binaryFieldInt, *binaryFieldInt) {
	a := newBianryFieldInt(ina.value)
	b := newBianryFieldInt(inb.value)
	zero := newBianryFieldInt(big.NewInt(0))

	x := newBianryFieldInt(big.NewInt(0))
	lastX := newBianryFieldInt(big.NewInt(1))
	y := newBianryFieldInt(big.NewInt(1))
	lastY := newBianryFieldInt(big.NewInt(0))
	for b.cmp(zero) != 0 {
		quot := newBianryFieldInt(a.value)
		quot.div(quot, b)

		{
			tmp := newBianryFieldInt(a.value)
			tmp.mod(a, b)
			a = b
			b = tmp
		}

		{
			tmp := newBianryFieldInt(x.value)
			tmp.mul(tmp, quot)
			tmp.add(tmp, lastX)
			lastX = x
			x = tmp
		}

		{
			tmp := newBianryFieldInt(y.value)
			tmp.mul(tmp, quot)
			tmp.add(tmp, lastY)
			lastY = y
			y = tmp
		}
	}

	return lastX, lastY
}
