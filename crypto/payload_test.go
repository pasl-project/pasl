package crypto

import (
	"bytes"
	"errors"
	"testing"
)

func PayloadKeyType(typeID uint16) error {
	payload := []byte("some test payload")
	key, err := NewKeyByType(typeID)
	if err != nil {
		return err
	}

	public, err := PublicFromBase58(key.Public.ToBase58())
	if err != nil {
		return err
	}

	encrypted, err := Encrypt(public.Convert(), payload)
	if err != nil {
		return err
	}
	if bytes.Equal(payload, encrypted) {
		return err
	}

	decrypted, err := Decrypt(key, encrypted)
	if err != nil {
		return err
	}

	if !bytes.Equal(payload, decrypted) {
		return errors.New("invlalid decryption result")
	}

	return nil
}

func TestPayload(t *testing.T) {
	if err := PayloadKeyType(NIDsecp256k1); err != nil {
		t.Fatal(err)
	}
	// if err := PayloadKeyType(NIDsect283k1); err != nil {
	// 	t.Fatal(err)
	// }
	if err := PayloadKeyType(NIDsecp384r1); err != nil {
		t.Fatal(err)
	}
	if err := PayloadKeyType(NIDsecp521r1); err != nil {
		t.Fatal(err)
	}
	if err := PayloadKeyType(0); err == nil {
		t.FailNow()
	}
}
