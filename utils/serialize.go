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

package utils

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
)

type Serializable interface {
	Serialize(io.Writer) error
	Deserialize(io.Reader) error
}

type BytesWithoutLengthPrefix struct {
	Bytes []byte
}

func (this *BytesWithoutLengthPrefix) Serialize(w io.Writer) error {
	_, err := w.Write(this.Bytes)
	return err
}

func (this *BytesWithoutLengthPrefix) Deserialize(r io.Reader) error {
	_, err := r.Read(this.Bytes[:])
	return err
}

func SerializeBytes(data []byte) []byte {
	dataLen := len(data)
	serialized := make([]byte, 2+dataLen)
	serialized[0] = (byte)(dataLen & 0xFF)
	serialized[1] = (byte)(dataLen >> 8)
	copy(serialized[2:], data)
	return serialized
}

func DeserializeBytes(reader io.Reader) (data []byte, err error) {
	var size int16
	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return
	}
	data = make([]byte, size)
	if err = binary.Read(reader, binary.LittleEndian, &data); err != nil {
		return nil, err
	}
	return
}

type pair struct {
	a interface{}
	b int
}

func strucWalker(struc interface{}, callback func(*reflect.Value) error) error {
	v := reflect.ValueOf(struc)
	if reflect.TypeOf(struc).Kind() == reflect.Ptr {
		v = v.Elem()
	}

	wayBack := list.New()
	wayBack.PushBack(pair{
		a: v,
		b: 0,
	})

	step := func(i int, v reflect.Value, el reflect.Value) (bool, error) {
		switch kind := el.Kind(); kind {
		case reflect.Struct:
			if el.CanAddr() {
				if _, ok := el.Addr().Interface().(Serializable); ok {
					if err := callback(&el); err != nil {
						return false, err
					}
					break
				}
			}
			wayBack.PushBack(pair{
				a: v,
				b: i + 1,
			})
			wayBack.PushBack(pair{
				a: el,
				b: 0,
			})
			return false, nil
		case reflect.Slice:
			switch el.Type().Elem().Kind() {
			case reflect.Uint8:
				if err := callback(&el); err != nil {
					return false, err
				}
			default:
				if err := callback(&el); err != nil {
					return false, err
				}
				wayBack.PushBack(pair{
					a: v,
					b: i + 1,
				})
				wayBack.PushBack(pair{
					a: el,
					b: 0,
				})
				return false, nil
			}
		default:
			if err := callback(&el); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	for wayBack.Len() > 0 {
		el := wayBack.Back()
		current := el.Value
		wayBack.Remove(el)

		v := current.(pair).a.(reflect.Value)

		switch kind := v.Kind(); kind {
		case reflect.Struct:
			if v.CanAddr() {
				if _, ok := v.Addr().Interface().(Serializable); ok {
					if err := callback(&v); err != nil {
						return err
					}
					break
				}
			}
			total := v.NumField()
			for i := current.(pair).b; i < total; i++ {
				cont, err := step(i, v, v.Field(i))
				if err != nil {
					return err
				}
				if !cont {
					break
				}
			}
		case reflect.Slice:
			total := v.Len()
			for i := current.(pair).b; i < total; i++ {
				cont, err := step(i, v, v.Index(i))
				if err != nil {
					return err
				}
				if !cont {
					break
				}
			}
		default:
			if err := callback(&v); err != nil {
				return err
			}
		}
	}

	return nil
}

func Serialize(struc interface{}) []byte {
	serialized := &bytes.Buffer{}

	// TODO: add error return value
	if strucWalker(struc, func(value *reflect.Value) error {
		switch kind := value.Kind(); kind {
		case reflect.Ptr:
			if err := value.Interface().(Serializable).Serialize(serialized); err != nil {
				return fmt.Errorf("Custom type serialization failed: %v", err)
			}
		case reflect.Interface:
			if err := value.Interface().(Serializable).Serialize(serialized); err != nil {
				return fmt.Errorf("Custom type serialization failed: %v", err)
			}
		case reflect.Struct:
			if err := value.Addr().Interface().(Serializable).Serialize(serialized); err != nil {
				return fmt.Errorf("Custom type serialization failed: %v", err)
			}
		case reflect.Bool:
			var val uint8
			if value.Bool() == true {
				val = 1
			} else {
				val = 0
			}
			binary.Write(serialized, binary.LittleEndian, val)
		case reflect.Uint8:
			binary.Write(serialized, binary.LittleEndian, uint8(value.Uint()))
		case reflect.Uint16:
			binary.Write(serialized, binary.LittleEndian, uint16(value.Uint()))
		case reflect.Uint32:
			binary.Write(serialized, binary.LittleEndian, uint32(value.Uint()))
		case reflect.Uint64:
			binary.Write(serialized, binary.LittleEndian, uint64(value.Uint()))
		case reflect.String:
			value := value.String()
			binary.Write(serialized, binary.LittleEndian, uint16(len(value)))
			binary.Write(serialized, binary.LittleEndian, []byte(value))
		case reflect.Slice:
			switch value.Type().Elem().Kind() {
			case reflect.Uint8:
				value := value.Bytes()
				binary.Write(serialized, binary.LittleEndian, uint16(len(value)))
				binary.Write(serialized, binary.LittleEndian, value)
			default:
				binary.Write(serialized, binary.LittleEndian, uint32(value.Len()))
			}
		case reflect.Array:
			switch value.Type().Elem().Kind() {
			case reflect.Uint8:
				binary.Write(serialized, binary.LittleEndian, value.Interface())
			default:
				return fmt.Errorf("Unimplemented %v %v", kind, value.Type().Elem().Kind())
			}
		default:
			return fmt.Errorf("Unimplemented %v", kind)
		}
		return nil
	}) != nil {
		return nil
	}

	return serialized.Bytes()
}

func Deserialize(struc interface{}, r io.Reader) error {
	return strucWalker(struc, func(value *reflect.Value) error {
		switch kind := value.Kind(); kind {
		case reflect.Ptr:
			if err := value.Interface().(Serializable).Deserialize(r); err != nil {
				return fmt.Errorf("Custom type deserialization failed: %v %v", err, value.Addr().Interface())
			}
		case reflect.Struct:
			if err := value.Addr().Interface().(Serializable).Deserialize(r); err != nil {
				return fmt.Errorf("Custom type deserialization failed: %v %v", err, value.Addr().Interface())
			}
		case reflect.Bool:
			var val uint8
			binary.Read(r, binary.LittleEndian, &val)
			if val == 0 {
				value.SetBool(false)
			} else {
				value.SetBool(true)
			}
		case reflect.Uint8:
			var val uint8
			binary.Read(r, binary.LittleEndian, &val)
			value.SetUint(uint64(val))
		case reflect.Uint16:
			var val uint16
			binary.Read(r, binary.LittleEndian, &val)
			value.SetUint(uint64(val))
		case reflect.Uint32:
			var val uint32
			binary.Read(r, binary.LittleEndian, &val)
			value.SetUint(uint64(val))
		case reflect.Uint64:
			var val uint64
			binary.Read(r, binary.LittleEndian, &val)
			value.SetUint(val)
		case reflect.String:
			var len uint16
			binary.Read(r, binary.LittleEndian, &len)
			var str []byte = make([]byte, len)
			if _, err := io.ReadFull(r, str); err != nil {
				return fmt.Errorf("insufficient data %v", err)
			}
			value.SetString(string(str))
		case reflect.Slice:
			switch kind := value.Type().Elem().Kind(); kind {
			case reflect.Uint8:
				var len uint16
				binary.Read(r, binary.LittleEndian, &len)
				data := make([]byte, len)
				if _, err := io.ReadFull(r, data); err != nil {
					return fmt.Errorf("insufficient data %v", err)
				}
				value.SetBytes(data)
			default:
				var len uint32
				binary.Read(r, binary.LittleEndian, &len)
				value.Set(reflect.MakeSlice(value.Type(), int(len), int(len)))
			}
		case reflect.Array:
			switch kind := value.Type().Elem().Kind(); kind {
			case reflect.Uint8:
				binary.Read(r, binary.LittleEndian, value.Addr().Interface())
			default:
				return fmt.Errorf("Unimplemented array %v", kind)
			}
		default:
			return fmt.Errorf("Unimplemented %v", kind)
		}
		return nil
	})
}
