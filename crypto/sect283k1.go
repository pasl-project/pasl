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

package crypto

import (
	"crypto/elliptic"
	"math/big"
	"sync"

	"github.com/xiphon/ellipticbinary"
)

var initonce sync.Once
var sect283k1 *ellipticbinary.Curve

func initSect283k1() {
	sect283k1 = &ellipticbinary.Curve{}
	sect283k1.Name = "sect283k1"
	sect283k1.P, _ = new(big.Int).SetString("0800000000000000000000000000000000000000000000000000000000000000000010a1", 16)
	sect283k1.N, _ = new(big.Int).SetString("01FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE9AE2ED07577265DFF7F94451E061E163C61", 16)
	sect283k1.A, _ = new(big.Int).SetString("0", 10)
	sect283k1.B, _ = new(big.Int).SetString("1", 10)
	sect283k1.Gx, _ = new(big.Int).SetString("0503213f78ca44883f1a3b8162f188e553cd265f23c1567a16876913b0c2ac2458492836", 16)
	sect283k1.Gy, _ = new(big.Int).SetString("01ccda380f1c9e318d90f95d07e5426fe87e45c0e8184698e45962364e34116177dd2259", 16)
	sect283k1.BitSize = 283
}

func Sect283k1() elliptic.Curve {
	initonce.Do(initSect283k1)
	return sect283k1
}
