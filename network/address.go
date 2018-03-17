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

package network

import (
	"fmt"
)

type AddressType int

const (
	TCP AddressType = iota
)

type Address interface {
	String() string
	GetType() AddressType
}

type AddressTcp struct {
	host     string
	port     uint16
	endpoint string
}

func NewAddressTcp(host string, port uint16) *AddressTcp {
	return &AddressTcp{
		host:     host,
		port:     port,
		endpoint: fmt.Sprintf("tcp://%s:%d", host, port),
	}
}

func (this *AddressTcp) GetType() AddressType {
	return AddressType(TCP)
}

func (this *AddressTcp) String() string {
	return this.endpoint
}
