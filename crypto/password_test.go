package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestValidDecrypt(t *testing.T) {
	encrypted, _ := hex.DecodeString("53616c7465645f5f14aa83a8b0c131ca186e873f5adbf846591b2de1f52293edf7e9e97804d98f30ae51306d2f70e7cede9bd8ccaabd3d97af93d899bb39031c")
	password := "test password"
	public := "3GhhbosoBSj7MRwpkU5K6of8cuc5C6HSQcZfsmdWqpQRgdTWSbthagZ1j1TesGgqmwrpmgDxgtaaLcuSLtfNVBGx3ms2NQuZDDTGck"

	decrypted := PasswordDecrypt(bytes.NewBuffer(encrypted), []byte(password))
	if decrypted == nil {
		t.FailNow()
	}

	private, err := UnmarshalPrivate(bytes.NewBuffer(decrypted))
	if err != nil {
		t.Fatal(err)
	}
	if private.Public.ToBase58() != public {
		t.FailNow()
	}

	buffer := bytes.NewBuffer(nil)
	if err := MarshalPrivate(buffer, private); err != nil {
		t.Fatal(err)
	}
	serialized := buffer.Bytes()
	if !bytes.Equal(serialized, decrypted[:len(serialized)]) {
		t.FailNow()
	}
	buffer = bytes.NewBuffer(nil)
	if err := PasswordEncrypt(buffer, serialized, []byte(password)); err != nil {
		t.Fatal(err)
	}
	decrypted = PasswordDecrypt(bytes.NewBuffer(encrypted), []byte(password))
	if decrypted == nil {
		t.FailNow()
	}
	private, err = UnmarshalPrivate(bytes.NewBuffer(decrypted))
	if err != nil {
		t.Fatal(err)
	}
	if private.Public.ToBase58() != public {
		t.FailNow()
	}
}

func TestInvalidDecrypt(t *testing.T) {
	if PasswordDecrypt(hex.NewDecoder(bytes.NewBuffer([]byte("53"))), nil) != nil {
		t.FailNow()
	}
	if PasswordDecrypt(hex.NewDecoder(bytes.NewBuffer([]byte("53616c7465645f00"))), nil) != nil {
		t.FailNow()
	}
	if PasswordDecrypt(hex.NewDecoder(bytes.NewBuffer([]byte("53616c7465645f5f14aa83a8b0c131"))), nil) != nil {
		t.FailNow()
	}

	empty := PasswordDecrypt(hex.NewDecoder(bytes.NewBuffer([]byte("53616c7465645f5f14aa83a8b0c131ca"))), nil)
	if _, err := UnmarshalPrivate(bytes.NewBuffer(empty)); err == nil {
		t.Fatal(err)
	}

	invalid := PasswordDecrypt(hex.NewDecoder(bytes.NewBuffer([]byte("53616c7465645f5f14aa83a8b0c131ca00"))), nil)
	if _, err := UnmarshalPrivate(bytes.NewBuffer(invalid)); err == nil {
		t.Fatal(err)
	}
}
