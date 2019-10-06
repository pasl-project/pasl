package wallet

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/pasl-project/pasl/crypto"
)

var (
	ErrLocked = errors.New("Wallet is locked")
)

type Key struct {
	encrypted []byte
	lock      sync.RWMutex
	private   *crypto.Key
}

func NewKey(encrypted []byte) *Key {
	return &Key{
		encrypted: encrypted,
		private:   nil,
	}
}

func (k *Key) Empty() bool {
	k.lock.RLock()
	defer k.lock.RUnlock()
	return k.encrypted == nil
}

func (k *Key) GetEncryptedHex() string {
	k.lock.RLock()
	defer k.lock.RUnlock()

	if k.encrypted == nil {
		return ""
	}
	return hex.EncodeToString(k.encrypted)
}

func (k *Key) Lock() {
	k.lock.Lock()
	defer k.lock.Unlock()
	k.private = nil
}

func (k *Key) Unlock(passphrase []byte) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	if k.encrypted == nil {
		return fmt.Errorf("empty key")
	}

	decrypted := crypto.PasswordDecrypt(bytes.NewBuffer(k.encrypted), passphrase)
	if decrypted == nil {
		return fmt.Errorf("failed to decrypt private key")
	}

	private, err := crypto.UnmarshalPrivate(bytes.NewBuffer(decrypted))
	if err != nil {
		return fmt.Errorf("failed to init private key: %v", err)
	}

	k.private = private
	return nil
}

func (k *Key) Update(encrypted []byte) {
	k.lock.Lock()
	defer k.lock.Unlock()
	k.encrypted = encrypted
}

func (k *Key) With(fn func(*crypto.Key) error) error {
	k.lock.RLock()
	defer k.lock.RUnlock()

	if k.private == nil {
		return ErrLocked
	}

	return fn(k.private)
}
