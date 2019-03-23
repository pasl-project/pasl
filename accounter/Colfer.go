package accounter

// Code generated by colf(1); DO NOT EDIT.
// The compiler used schema file accounter\serialization.colf.

import (
	"encoding/binary"
	"fmt"
	"io"
)

var intconv = binary.BigEndian

// Colfer configuration attributes
var (
	// ColferSizeMax is the upper limit for serial byte sizes.
	ColferSizeMax = 16 * 1024 * 1024
	// ColferListMax is the upper limit for the number of elements in a list.
	ColferListMax = 64 * 1024
)

// ColferMax signals an upper limit breach.
type ColferMax string

// Error honors the error interface.
func (m ColferMax) Error() string { return string(m) }

// ColferError signals a data mismatch as as a byte index.
type ColferError int

// Error honors the error interface.
func (i ColferError) Error() string {
	return fmt.Sprintf("colfer: unknown header at byte %d", i)
}

// ColferTail signals data continuation as a byte index.
type ColferTail int

// Error honors the error interface.
func (i ColferTail) Error() string {
	return fmt.Sprintf("colfer: data continuation at byte %d", i)
}

type PublicPod struct {
	TypeId uint16

	X []byte

	Y []byte
}

// MarshalTo encodes o as Colfer into buf and returns the number of bytes written.
// If the buffer is too small, MarshalTo will panic.
func (o *PublicPod) MarshalTo(buf []byte) int {
	var i int

	if x := o.TypeId; x >= 1<<8 {
		buf[i] = 0
		i++
		buf[i] = byte(x >> 8)
		i++
		buf[i] = byte(x)
		i++
	} else if x != 0 {
		buf[i] = 0 | 0x80
		i++
		buf[i] = byte(x)
		i++
	}

	if l := len(o.X); l != 0 {
		buf[i] = 1
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		i += copy(buf[i:], o.X)
	}

	if l := len(o.Y); l != 0 {
		buf[i] = 2
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		i += copy(buf[i:], o.Y)
	}

	buf[i] = 0x7f
	i++
	return i
}

// MarshalLen returns the Colfer serial byte size.
// The error return option is accounter.ColferMax.
func (o *PublicPod) MarshalLen() (int, error) {
	l := 1

	if x := o.TypeId; x >= 1<<8 {
		l += 3
	} else if x != 0 {
		l += 2
	}

	if x := len(o.X); x != 0 {
		if x > ColferSizeMax {
			return 0, ColferMax(fmt.Sprintf("colfer: field accounter.PublicPod.X exceeds %d bytes", ColferSizeMax))
		}
		for l += x + 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if x := len(o.Y); x != 0 {
		if x > ColferSizeMax {
			return 0, ColferMax(fmt.Sprintf("colfer: field accounter.PublicPod.Y exceeds %d bytes", ColferSizeMax))
		}
		for l += x + 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if l > ColferSizeMax {
		return l, ColferMax(fmt.Sprintf("colfer: struct accounter.PublicPod exceeds %d bytes", ColferSizeMax))
	}
	return l, nil
}

// MarshalBinary encodes o as Colfer conform encoding.BinaryMarshaler.
// The error return option is accounter.ColferMax.
func (o *PublicPod) MarshalBinary() (data []byte, err error) {
	l, err := o.MarshalLen()
	if err != nil {
		return nil, err
	}
	data = make([]byte, l)
	o.MarshalTo(data)
	return data, nil
}

// Unmarshal decodes data as Colfer and returns the number of bytes read.
// The error return options are io.EOF, accounter.ColferError and accounter.ColferMax.
func (o *PublicPod) Unmarshal(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.EOF
	}
	header := data[0]
	i := 1

	if header == 0 {
		start := i
		i += 2
		if i >= len(data) {
			goto eof
		}
		o.TypeId = intconv.Uint16(data[start:])
		header = data[i]
		i++
	} else if header == 0|0x80 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		o.TypeId = uint16(data[start])
		header = data[i]
		i++
	}

	if header == 1 {
		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}

		if x > uint(ColferSizeMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: accounter.PublicPod.X size %d exceeds %d bytes", x, ColferSizeMax))
		}
		v := make([]byte, int(x))

		start := i
		i += len(v)
		if i >= len(data) {
			goto eof
		}
		copy(v, data[start:i])
		o.X = v

		header = data[i]
		i++
	}

	if header == 2 {
		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}

		if x > uint(ColferSizeMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: accounter.PublicPod.Y size %d exceeds %d bytes", x, ColferSizeMax))
		}
		v := make([]byte, int(x))

		start := i
		i += len(v)
		if i >= len(data) {
			goto eof
		}
		copy(v, data[start:i])
		o.Y = v

		header = data[i]
		i++
	}

	if header != 0x7f {
		return 0, ColferError(i - 1)
	}
	if i < ColferSizeMax {
		return i, nil
	}
eof:
	if i >= ColferSizeMax {
		return 0, ColferMax(fmt.Sprintf("colfer: struct accounter.PublicPod size exceeds %d bytes", ColferSizeMax))
	}
	return 0, io.EOF
}

// UnmarshalBinary decodes data as Colfer conform encoding.BinaryUnmarshaler.
// The error return options are io.EOF, accounter.ColferError, accounter.ColferTail and accounter.ColferMax.
func (o *PublicPod) UnmarshalBinary(data []byte) error {
	i, err := o.Unmarshal(data)
	if i < len(data) && err == nil {
		return ColferTail(i)
	}
	return err
}

type AccountPod struct {
	Number uint32

	PublicKey *PublicPod

	Balance uint64

	UpdatedIndex uint32

	Operations uint32

	OperationsTotal uint32

	Timestamp uint32
}

// MarshalTo encodes o as Colfer into buf and returns the number of bytes written.
// If the buffer is too small, MarshalTo will panic.
func (o *AccountPod) MarshalTo(buf []byte) int {
	var i int

	if x := o.Number; x >= 1<<21 {
		buf[i] = 0 | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = 0
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	if v := o.PublicKey; v != nil {
		buf[i] = 1
		i++
		i += v.MarshalTo(buf[i:])
	}

	if x := o.Balance; x >= 1<<49 {
		buf[i] = 2 | 0x80
		intconv.PutUint64(buf[i+1:], x)
		i += 9
	} else if x != 0 {
		buf[i] = 2
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	if x := o.UpdatedIndex; x >= 1<<21 {
		buf[i] = 3 | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = 3
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	if x := o.Operations; x >= 1<<21 {
		buf[i] = 4 | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = 4
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	if x := o.OperationsTotal; x >= 1<<21 {
		buf[i] = 5 | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = 5
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	if x := o.Timestamp; x >= 1<<21 {
		buf[i] = 6 | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = 6
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	buf[i] = 0x7f
	i++
	return i
}

// MarshalLen returns the Colfer serial byte size.
// The error return option is accounter.ColferMax.
func (o *AccountPod) MarshalLen() (int, error) {
	l := 1

	if x := o.Number; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if v := o.PublicKey; v != nil {
		vl, err := v.MarshalLen()
		if err != nil {
			return 0, err
		}
		l += vl + 1
	}

	if x := o.Balance; x >= 1<<49 {
		l += 9
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if x := o.UpdatedIndex; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if x := o.Operations; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if x := o.OperationsTotal; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if x := o.Timestamp; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if l > ColferSizeMax {
		return l, ColferMax(fmt.Sprintf("colfer: struct accounter.AccountPod exceeds %d bytes", ColferSizeMax))
	}
	return l, nil
}

// MarshalBinary encodes o as Colfer conform encoding.BinaryMarshaler.
// The error return option is accounter.ColferMax.
func (o *AccountPod) MarshalBinary() (data []byte, err error) {
	l, err := o.MarshalLen()
	if err != nil {
		return nil, err
	}
	data = make([]byte, l)
	o.MarshalTo(data)
	return data, nil
}

// Unmarshal decodes data as Colfer and returns the number of bytes read.
// The error return options are io.EOF, accounter.ColferError and accounter.ColferMax.
func (o *AccountPod) Unmarshal(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.EOF
	}
	header := data[0]
	i := 1

	if header == 0 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.Number = x

		header = data[i]
		i++
	} else if header == 0|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.Number = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}

	if header == 1 {
		o.PublicKey = new(PublicPod)
		n, err := o.PublicKey.Unmarshal(data[i:])
		if err != nil {
			if err == io.EOF && len(data) >= ColferSizeMax {
				return 0, ColferMax(fmt.Sprintf("colfer: accounter.AccountPod size exceeds %d bytes", ColferSizeMax))
			}
			return 0, err
		}
		i += n

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
	}

	if header == 2 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint64(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint64(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 || shift == 56 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.Balance = x

		header = data[i]
		i++
	} else if header == 2|0x80 {
		start := i
		i += 8
		if i >= len(data) {
			goto eof
		}
		o.Balance = intconv.Uint64(data[start:])
		header = data[i]
		i++
	}

	if header == 3 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.UpdatedIndex = x

		header = data[i]
		i++
	} else if header == 3|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.UpdatedIndex = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}

	if header == 4 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.Operations = x

		header = data[i]
		i++
	} else if header == 4|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.Operations = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}

	if header == 5 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.OperationsTotal = x

		header = data[i]
		i++
	} else if header == 5|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.OperationsTotal = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}

	if header == 6 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.Timestamp = x

		header = data[i]
		i++
	} else if header == 6|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.Timestamp = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}

	if header != 0x7f {
		return 0, ColferError(i - 1)
	}
	if i < ColferSizeMax {
		return i, nil
	}
eof:
	if i >= ColferSizeMax {
		return 0, ColferMax(fmt.Sprintf("colfer: struct accounter.AccountPod size exceeds %d bytes", ColferSizeMax))
	}
	return 0, io.EOF
}

// UnmarshalBinary decodes data as Colfer conform encoding.BinaryUnmarshaler.
// The error return options are io.EOF, accounter.ColferError, accounter.ColferTail and accounter.ColferMax.
func (o *AccountPod) UnmarshalBinary(data []byte) error {
	i, err := o.Unmarshal(data)
	if i < len(data) && err == nil {
		return ColferTail(i)
	}
	return err
}

type PackPod struct {
	Accounts []*AccountPod

	CumulativeDifficulty []byte

	Dirty bool

	Hash []byte

	Index uint32
}

// MarshalTo encodes o as Colfer into buf and returns the number of bytes written.
// If the buffer is too small, MarshalTo will panic.
// All nil entries in o.Accounts will be replaced with a new value.
func (o *PackPod) MarshalTo(buf []byte) int {
	var i int

	if l := len(o.Accounts); l != 0 {
		buf[i] = 0
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		for vi, v := range o.Accounts {
			if v == nil {
				v = new(AccountPod)
				o.Accounts[vi] = v
			}
			i += v.MarshalTo(buf[i:])
		}
	}

	if l := len(o.CumulativeDifficulty); l != 0 {
		buf[i] = 1
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		i += copy(buf[i:], o.CumulativeDifficulty)
	}

	if o.Dirty {
		buf[i] = 2
		i++
	}

	if l := len(o.Hash); l != 0 {
		buf[i] = 3
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		i += copy(buf[i:], o.Hash)
	}

	if x := o.Index; x >= 1<<21 {
		buf[i] = 4 | 0x80
		intconv.PutUint32(buf[i+1:], x)
		i += 5
	} else if x != 0 {
		buf[i] = 4
		i++
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
	}

	buf[i] = 0x7f
	i++
	return i
}

// MarshalLen returns the Colfer serial byte size.
// The error return option is accounter.ColferMax.
func (o *PackPod) MarshalLen() (int, error) {
	l := 1

	if x := len(o.Accounts); x != 0 {
		if x > ColferListMax {
			return 0, ColferMax(fmt.Sprintf("colfer: field accounter.PackPod.Accounts exceeds %d elements", ColferListMax))
		}
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
		for _, v := range o.Accounts {
			if v == nil {
				l++
				continue
			}
			vl, err := v.MarshalLen()
			if err != nil {
				return 0, err
			}
			l += vl
		}
		if l > ColferSizeMax {
			return 0, ColferMax(fmt.Sprintf("colfer: struct accounter.PackPod size exceeds %d bytes", ColferSizeMax))
		}
	}

	if x := len(o.CumulativeDifficulty); x != 0 {
		if x > ColferSizeMax {
			return 0, ColferMax(fmt.Sprintf("colfer: field accounter.PackPod.CumulativeDifficulty exceeds %d bytes", ColferSizeMax))
		}
		for l += x + 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if o.Dirty {
		l++
	}

	if x := len(o.Hash); x != 0 {
		if x > ColferSizeMax {
			return 0, ColferMax(fmt.Sprintf("colfer: field accounter.PackPod.Hash exceeds %d bytes", ColferSizeMax))
		}
		for l += x + 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if x := o.Index; x >= 1<<21 {
		l += 5
	} else if x != 0 {
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
	}

	if l > ColferSizeMax {
		return l, ColferMax(fmt.Sprintf("colfer: struct accounter.PackPod exceeds %d bytes", ColferSizeMax))
	}
	return l, nil
}

// MarshalBinary encodes o as Colfer conform encoding.BinaryMarshaler.
// All nil entries in o.Accounts will be replaced with a new value.
// The error return option is accounter.ColferMax.
func (o *PackPod) MarshalBinary() (data []byte, err error) {
	l, err := o.MarshalLen()
	if err != nil {
		return nil, err
	}
	data = make([]byte, l)
	o.MarshalTo(data)
	return data, nil
}

// Unmarshal decodes data as Colfer and returns the number of bytes read.
// The error return options are io.EOF, accounter.ColferError and accounter.ColferMax.
func (o *PackPod) Unmarshal(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.EOF
	}
	header := data[0]
	i := 1

	if header == 0 {
		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}

		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: accounter.PackPod.Accounts length %d exceeds %d elements", x, ColferListMax))
		}

		l := int(x)
		a := make([]*AccountPod, l)
		malloc := make([]AccountPod, l)
		for ai := range a {
			v := &malloc[ai]
			a[ai] = v

			n, err := v.Unmarshal(data[i:])
			if err != nil {
				if err == io.EOF && len(data) >= ColferSizeMax {
					return 0, ColferMax(fmt.Sprintf("colfer: accounter.PackPod size exceeds %d bytes", ColferSizeMax))
				}
				return 0, err
			}
			i += n
		}
		o.Accounts = a

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
	}

	if header == 1 {
		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}

		if x > uint(ColferSizeMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: accounter.PackPod.CumulativeDifficulty size %d exceeds %d bytes", x, ColferSizeMax))
		}
		v := make([]byte, int(x))

		start := i
		i += len(v)
		if i >= len(data) {
			goto eof
		}
		copy(v, data[start:i])
		o.CumulativeDifficulty = v

		header = data[i]
		i++
	}

	if header == 2 {
		if i >= len(data) {
			goto eof
		}
		o.Dirty = true
		header = data[i]
		i++
	}

	if header == 3 {
		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}

		if x > uint(ColferSizeMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: accounter.PackPod.Hash size %d exceeds %d bytes", x, ColferSizeMax))
		}
		v := make([]byte, int(x))

		start := i
		i += len(v)
		if i >= len(data) {
			goto eof
		}
		copy(v, data[start:i])
		o.Hash = v

		header = data[i]
		i++
	}

	if header == 4 {
		start := i
		i++
		if i >= len(data) {
			goto eof
		}
		x := uint32(data[start])

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				b := uint32(data[i])
				i++
				if i >= len(data) {
					goto eof
				}

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}
		o.Index = x

		header = data[i]
		i++
	} else if header == 4|0x80 {
		start := i
		i += 4
		if i >= len(data) {
			goto eof
		}
		o.Index = intconv.Uint32(data[start:])
		header = data[i]
		i++
	}

	if header != 0x7f {
		return 0, ColferError(i - 1)
	}
	if i < ColferSizeMax {
		return i, nil
	}
eof:
	if i >= ColferSizeMax {
		return 0, ColferMax(fmt.Sprintf("colfer: struct accounter.PackPod size exceeds %d bytes", ColferSizeMax))
	}
	return 0, io.EOF
}

// UnmarshalBinary decodes data as Colfer conform encoding.BinaryUnmarshaler.
// The error return options are io.EOF, accounter.ColferError, accounter.ColferTail and accounter.ColferMax.
func (o *PackPod) UnmarshalBinary(data []byte) error {
	i, err := o.Unmarshal(data)
	if i < len(data) && err == nil {
		return ColferTail(i)
	}
	return err
}

type AccounterPod struct {
	Packs []*PackPod
}

// MarshalTo encodes o as Colfer into buf and returns the number of bytes written.
// If the buffer is too small, MarshalTo will panic.
// All nil entries in o.Packs will be replaced with a new value.
func (o *AccounterPod) MarshalTo(buf []byte) int {
	var i int

	if l := len(o.Packs); l != 0 {
		buf[i] = 0
		i++
		x := uint(l)
		for x >= 0x80 {
			buf[i] = byte(x | 0x80)
			x >>= 7
			i++
		}
		buf[i] = byte(x)
		i++
		for vi, v := range o.Packs {
			if v == nil {
				v = new(PackPod)
				o.Packs[vi] = v
			}
			i += v.MarshalTo(buf[i:])
		}
	}

	buf[i] = 0x7f
	i++
	return i
}

// MarshalLen returns the Colfer serial byte size.
// The error return option is accounter.ColferMax.
func (o *AccounterPod) MarshalLen() (int, error) {
	l := 1

	if x := len(o.Packs); x != 0 {
		if x > ColferListMax {
			return 0, ColferMax(fmt.Sprintf("colfer: field accounter.AccounterPod.Packs exceeds %d elements", ColferListMax))
		}
		for l += 2; x >= 0x80; l++ {
			x >>= 7
		}
		for _, v := range o.Packs {
			if v == nil {
				l++
				continue
			}
			vl, err := v.MarshalLen()
			if err != nil {
				return 0, err
			}
			l += vl
		}
		if l > ColferSizeMax {
			return 0, ColferMax(fmt.Sprintf("colfer: struct accounter.AccounterPod size exceeds %d bytes", ColferSizeMax))
		}
	}

	if l > ColferSizeMax {
		return l, ColferMax(fmt.Sprintf("colfer: struct accounter.AccounterPod exceeds %d bytes", ColferSizeMax))
	}
	return l, nil
}

// MarshalBinary encodes o as Colfer conform encoding.BinaryMarshaler.
// All nil entries in o.Packs will be replaced with a new value.
// The error return option is accounter.ColferMax.
func (o *AccounterPod) MarshalBinary() (data []byte, err error) {
	l, err := o.MarshalLen()
	if err != nil {
		return nil, err
	}
	data = make([]byte, l)
	o.MarshalTo(data)
	return data, nil
}

// Unmarshal decodes data as Colfer and returns the number of bytes read.
// The error return options are io.EOF, accounter.ColferError and accounter.ColferMax.
func (o *AccounterPod) Unmarshal(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.EOF
	}
	header := data[0]
	i := 1

	if header == 0 {
		if i >= len(data) {
			goto eof
		}
		x := uint(data[i])
		i++

		if x >= 0x80 {
			x &= 0x7f
			for shift := uint(7); ; shift += 7 {
				if i >= len(data) {
					goto eof
				}
				b := uint(data[i])
				i++

				if b < 0x80 {
					x |= b << shift
					break
				}
				x |= (b & 0x7f) << shift
			}
		}

		if x > uint(ColferListMax) {
			return 0, ColferMax(fmt.Sprintf("colfer: accounter.AccounterPod.Packs length %d exceeds %d elements", x, ColferListMax))
		}

		l := int(x)
		a := make([]*PackPod, l)
		malloc := make([]PackPod, l)
		for ai := range a {
			v := &malloc[ai]
			a[ai] = v

			n, err := v.Unmarshal(data[i:])
			if err != nil {
				if err == io.EOF && len(data) >= ColferSizeMax {
					return 0, ColferMax(fmt.Sprintf("colfer: accounter.AccounterPod size exceeds %d bytes", ColferSizeMax))
				}
				return 0, err
			}
			i += n
		}
		o.Packs = a

		if i >= len(data) {
			goto eof
		}
		header = data[i]
		i++
	}

	if header != 0x7f {
		return 0, ColferError(i - 1)
	}
	if i < ColferSizeMax {
		return i, nil
	}
eof:
	if i >= ColferSizeMax {
		return 0, ColferMax(fmt.Sprintf("colfer: struct accounter.AccounterPod size exceeds %d bytes", ColferSizeMax))
	}
	return 0, io.EOF
}

// UnmarshalBinary decodes data as Colfer conform encoding.BinaryUnmarshaler.
// The error return options are io.EOF, accounter.ColferError, accounter.ColferTail and accounter.ColferMax.
func (o *AccounterPod) UnmarshalBinary(data []byte) error {
	i, err := o.Unmarshal(data)
	if i < len(data) && err == nil {
		return ColferTail(i)
	}
	return err
}
