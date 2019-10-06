package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/pasl-project/pasl/utils"
)

const saltMagic = "Salted__"

type PrivateSerialized struct {
	TypeID  uint16
	Private []byte
}

func PasswordEncrypt(w io.Writer, data []byte, key []byte) error {
	if _, err := w.Write([]byte(saltMagic)); err != nil {
		return err
	}
	salt := [len(saltMagic)]byte{}
	if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
		return nil
	}
	if _, err := w.Write(salt[:]); err != nil {
		return err
	}

	h := sha256.New()
	if _, err := h.Write(key); err != nil {
		return nil
	}
	if _, err := h.Write(salt[:]); err != nil {
		return nil
	}
	encryptionKey := h.Sum(nil)

	h.Reset()
	if _, err := h.Write(encryptionKey); err != nil {
		return nil
	}
	if _, err := h.Write(key); err != nil {
		return nil
	}
	if _, err := h.Write(salt[:]); err != nil {
		return nil
	}
	iv := h.Sum(nil)[:16]

	encrypted, err := aes256Cbc(data[:], encryptionKey, iv, true)
	if err != nil {
		return err
	}
	_, err = w.Write(encrypted)
	return err
}

func PasswordDecrypt(encrypted io.Reader, key []byte) []byte {
	salt := [len(saltMagic)]byte{}
	if _, err := io.ReadFull(encrypted, salt[:]); err != nil {
		return nil
	}
	if !bytes.Equal(salt[:], []byte(saltMagic)) {
		return nil
	}
	if _, err := io.ReadFull(encrypted, salt[:]); err != nil {
		return nil
	}

	h := sha256.New()
	if _, err := h.Write(key); err != nil {
		return nil
	}
	if _, err := h.Write(salt[:]); err != nil {
		return nil
	}
	encryptionKey := h.Sum(nil)

	h.Reset()
	if _, err := h.Write(encryptionKey); err != nil {
		return nil
	}
	if _, err := h.Write(key); err != nil {
		return nil
	}
	if _, err := h.Write(salt[:]); err != nil {
		return nil
	}
	iv := h.Sum(nil)[:16]

	data, err := ioutil.ReadAll(encrypted)
	if err != nil {
		return nil
	}
	if decrypted, err := aes256Cbc(data[:], encryptionKey, iv, false); err == nil {
		return decrypted
	}
	return nil
}

func MarshalPrivate(w io.Writer, private *Key) error {
	serialized := utils.Serialize(PrivateSerialized{
		TypeID:  private.Public.TypeId,
		Private: private.Private,
	})
	if serialized == nil {
		return fmt.Errorf("failed to serialize private key")
	}
	_, err := w.Write(serialized)
	return err
}

func UnmarshalPrivate(r io.Reader) (*Key, error) {
	header := PrivateSerialized{}
	if err := utils.Deserialize(&header, r); err != nil {
		return nil, err
	}
	return NewKeyFromPrivate(header.TypeID, header.Private)
}
