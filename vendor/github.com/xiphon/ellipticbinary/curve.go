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

// Package ellipticbinary provides basic Elliptic Curve primitives over Binary Field GF(2ⁿ)
package ellipticbinary

import (
	"crypto/elliptic"
	"math/big"
)

// Curve defines Elliptic Curve parameters
type Curve struct {
	elliptic.Curve

	elliptic.CurveParams
	A *big.Int
}

func (this *Curve) Params() *elliptic.CurveParams {
	return &this.CurveParams
}

// IsOnCurve check whether an arbitrary point belongs to the curve
func (this *Curve) IsOnCurve(x, y *big.Int) bool {
	// y² + xy = x³ + Ax² + B

	yVal := newBianryFieldInt(y)
	y2xy := newBianryFieldInt(y)
	{
		y2xy.mul(yVal, yVal) //y²

		xy := newBianryFieldInt(x) //xy
		xy.mul(xy, yVal)

		y2xy.add(y2xy, xy)                        //y² + xy
		y2xy.mod(y2xy, newBianryFieldInt(this.P)) //y² + xy mod P
	}

	xVal := newBianryFieldInt(x)

	x3ax2b := newBianryFieldInt(x)
	x3ax2b.mul(x3ax2b, xVal)

	ax2 := newBianryFieldInt(this.A)
	ax2.mul(x3ax2b, ax2)

	x3ax2b.mul(x3ax2b, xVal)                      //x³
	x3ax2b.add(x3ax2b, newBianryFieldInt(this.B)) //x³ + B
	x3ax2b.add(x3ax2b, ax2)                       //x³ + Ax² + B
	x3ax2b.mod(x3ax2b, newBianryFieldInt(this.P)) //x³ + Ax² + B mod P

	return y2xy.cmp(x3ax2b) == 0
}

// Add performs point addition
func (this *Curve) Add(x1, y1, x2, y2 *big.Int) (x, y *big.Int) {
	// TODO: Identity?
	if x1.BitLen() == 0 && y1.BitLen() == 0 {
		return x2, y2
	}
	if x2.BitLen() == 0 && y2.BitLen() == 0 {
		return x1, y1
	}

	y1y2 := newBianryFieldInt(y1)
	y1y2.add(y1y2, newBianryFieldInt(y2))

	x1Bin := newBianryFieldInt(x1)
	x2Bin := newBianryFieldInt(x2)
	x1x2 := newBianryFieldInt(big.NewInt(0)).add(x1Bin, x2Bin)

	s := newBianryFieldInt(big.NewInt(0)).divmod(y1y2, x1x2, newBianryFieldInt(this.P))

	xVal := newBianryFieldInt(big.NewInt(0)).mul(s, s)
	xVal.add(xVal, s)
	xVal.add(xVal, x1Bin)
	xVal.add(xVal, x2Bin)
	xVal.add(xVal, newBianryFieldInt(this.A))
	xVal.mod(xVal, newBianryFieldInt(this.P))

	yVal := newBianryFieldInt(x1)
	yVal.add(yVal, xVal)
	yVal.mul(yVal, s)
	yVal.add(yVal, xVal)
	yVal.add(yVal, newBianryFieldInt(y1))
	yVal.mod(yVal, newBianryFieldInt(this.P))

	return xVal.value, yVal.value
}

// Double does point doubling
func (this *Curve) Double(x1, y1 *big.Int) (x, y *big.Int) {
	xVal := newBianryFieldInt(x1)
	s := newBianryFieldInt(big.NewInt(0)).divmod(newBianryFieldInt(y1), xVal, newBianryFieldInt(this.P))
	s.add(s, xVal)

	xVal = newBianryFieldInt(big.NewInt(0)).mul(s, s)
	xVal.add(xVal, s)
	xVal.add(xVal, newBianryFieldInt(this.A))
	xVal.mod(xVal, newBianryFieldInt(this.P))

	x1x1 := newBianryFieldInt(x1)
	x1x1.mul(x1x1, x1x1)

	yVal := newBianryFieldInt(big.NewInt(1))
	yVal.add(yVal, s)
	yVal.mul(yVal, xVal)
	yVal.add(yVal, x1x1)
	yVal.mod(yVal, newBianryFieldInt(this.P))

	return xVal.value, yVal.value
}

// ScalarMult multiplies point P(x1, y1) by a scalar k represented in big-endian form
func (this *Curve) ScalarMult(x1, y1 *big.Int, k []byte) (x, y *big.Int) {
	doublerX := big.NewInt(0).Set(x1)
	doublerY := big.NewInt(0).Set(y1)
	num := big.NewInt(0).SetBytes(k)
	zero := big.NewInt(0)

	//TODO: Identity?
	accX := big.NewInt(0)
	accY := big.NewInt(0)

	for num.Cmp(zero) > 0 {
		if num.Bit(0) != 0 {
			accX, accY = this.Add(accX, accY, doublerX, doublerY)
		}
		num.Rsh(num, 1)
		doublerX, doublerY = this.Double(doublerX, doublerY)
	}

	return accX, accY
}

// ScalarBaseMult multiplies base point G by a scalar k represented in big-endian form
func (this *Curve) ScalarBaseMult(k []byte) (x, y *big.Int) {
	return this.ScalarMult(this.Gx, this.Gy, k)
}
