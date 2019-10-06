package wallet

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/network"
	"github.com/pasl-project/pasl/safebox/tx"
	"github.com/pasl-project/pasl/utils"

	"github.com/powerman/rpc-codec/jsonrpc2"
)

type Storage struct {
	PrivateKey string `json:"private"`
}

type Wallet struct {
	client *jsonrpc2.Client
	key    *Key
	write  func([]byte) error
}

func NewWallet(contents []byte, passphrase []byte, write func([]byte) error, coreRPCAddress string) (*Wallet, error) {
	w := &Wallet{
		client: jsonrpc2.NewHTTPClient("http://" + coreRPCAddress),
		key:    NewKey(nil),
		write:  write,
	}

	if len(contents) != 0 {
		var current Storage
		if err := json.Unmarshal(contents, &current); err != nil {
			return w, fmt.Errorf("wallet data is corrupted: %v", err)
		}

		return w, w.set(current.PrivateKey, passphrase)
	}

	return w, nil
}

func (w *Wallet) Close() error {
	return w.client.Close()
}

func (w *Wallet) GetHandlers() map[string]interface{} {
	return map[string]interface{}{
		"getwalletpubkey":        w.GetWalletPubKey,
		"getwalletaccounts":      w.GetWalletAccounts,
		"sendto":                 w.SendTo,
		"payloaddecrypt":         w.PayloadDecrypt,
		"payloadencrypt":         w.PayloadEncrypt,
		"getwalletaccountscount": w.GetWalletAccountsCount,
		"getwalletpubkeys":       w.GetWalletPubKeys,
		"changekey":              w.ChangeKey,
		"getprivatekeyencrypted": w.GetPrivateKeyEncrypted,
		"setprivatekey":          w.SetPrivateKey,
		"createwallet":           w.CreateWallet,
	}
}

func (w *Wallet) set(encryptedHex string, passphrase []byte) error {
	encrypted, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return fmt.Errorf("failed to decode private key: %v", err)
	}

	w.key.Update(encrypted)

	if len(passphrase) == 0 {
		return nil
	}

	return w.key.Unlock(passphrase)
}

func (w *Wallet) generate(passphrase []byte) error {
	if len(passphrase) == 0 {
		return fmt.Errorf("empty passphrase")
	}

	keyType := crypto.NIDsecp256k1
	key, err := crypto.NewKeyByType(keyType)
	if err != nil {
		return err
	}

	serialized := bytes.NewBuffer(nil)
	if err := crypto.MarshalPrivate(serialized, key); err != nil {
		return err
	}
	encrypted := bytes.NewBuffer(nil)
	if err := crypto.PasswordEncrypt(encrypted, serialized.Bytes(), []byte(passphrase)); err != nil {
		return err
	}

	if err := w.store(hex.EncodeToString(encrypted.Bytes())); err != nil {
		return err
	}

	w.key.Update(encrypted.Bytes())
	return nil
}

func (w *Wallet) store(privateKeyEncryptedHex string) error {
	current := Storage{
		PrivateKey: privateKeyEncryptedHex,
	}
	contents, err := json.Marshal(&current)
	if err != nil {
		return fmt.Errorf("failed to convert to JSON: %v", err)
	}
	if err = w.write(contents); err != nil {
		return fmt.Errorf("failed to write new key: %v", err)
	}
	return nil
}

func (w *Wallet) getPublic() (p *Public, err error) {
	err = w.key.With(func(private *crypto.Key) error {
		p = &Public{
			TypeID:    private.Public.TypeId,
			X:         hex.EncodeToString(private.Public.X.Bytes()),
			Y:         hex.EncodeToString(private.Public.Y.Bytes()),
			EncPubkey: hex.EncodeToString(utils.Serialize(private.Public.Serialized())),
			B58Pubkey: private.Public.ToBase58(),
		}
		return nil
	})
	return p, err
}

func (w *Wallet) GetWalletPubKey(context.Context) (*Public, error) {
	return w.getPublic()
}

func (w *Wallet) GetWalletPubKeys(context.Context) ([]*Public, error) {
	p, err := w.getPublic()
	if err != nil {
		return nil, err
	}
	return []*Public{p}, nil
}

func (w *Wallet) getWalletAccountNumbers() (map[uint32]struct{}, error) {
	var accounts []uint32

	var b58PubKey string
	err := w.key.With(func(private *crypto.Key) error {
		b58PubKey = private.Public.ToBase58()
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := w.client.Call("getpubkeyaccounts", struct {
		B58_pubkey string
	}{
		B58_pubkey: b58PubKey,
	}, &accounts); err != nil {
		return nil, err
	}

	result := make(map[uint32]struct{})
	for _, each := range accounts {
		result[each] = struct{}{}
	}

	return result, nil
}

func (w *Wallet) getAccount(number uint32) (*network.Account, error) {
	var account network.Account
	if err := w.client.Call("getaccount", struct {
		Account uint32
	}{
		Account: number,
	}, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

func (w *Wallet) GetPrivateKeyEncrypted(context.Context) (encryptedHex string, err error) {
	return w.key.GetEncryptedHex(), nil
}

func (w *Wallet) SetPrivateKey(_ context.Context, params *struct {
	EncryptedHex   string
	AllowOverwrite bool
}) error {
	if !w.key.Empty() && !params.AllowOverwrite {
		return fmt.Errorf("can't overwrite an existing wallet")
	}
	if err := w.set(params.EncryptedHex, nil); err != nil {
		return err
	}
	if err := w.store(params.EncryptedHex); err != nil {
		return err
	}
	return nil
}

func (w *Wallet) CreateWallet(_ context.Context, params *struct {
	Password       string
	AllowOverwrite bool
}) (bool, error) {
	if !w.key.Empty() && !params.AllowOverwrite {
		return false, fmt.Errorf("can't overwrite an existing wallet")
	}
	if err := w.generate([]byte(params.Password)); err != nil {
		return false, err
	}
	return true, nil
}

func (w *Wallet) GetWalletAccounts(context.Context) ([]Account, error) {
	accounts, err := w.getWalletAccountNumbers()
	if err != nil {
		return nil, err
	}

	public, err := w.getPublic()
	if err != nil {
		return nil, err
	}

	result := make([]Account, 0, len(accounts))
	for number := range accounts {
		result = append(result, Account{
			Public: public,
			Name:   fmt.Sprintf("%d", number),
			CanUse: true,
		})
	}

	return result, nil
}

func (w *Wallet) GetWalletAccountsCount(context.Context) (uint, error) {
	accounts, err := w.getWalletAccountNumbers()
	if err != nil {
		return 0, err
	}
	return uint(len(accounts)), nil
}

func (w *Wallet) PayloadEncrypt(_ context.Context, params *struct {
	Payload        string
	Payload_method string
	B58_pubkey     string
	Enc_pubkey     string
}) (result string, err error) {
	switch params.Payload_method {
	case "pubkey":
		{
			payload, err := hex.DecodeString(params.Payload)
			if err != nil {
				return "", err
			}
			public, err := crypto.PublicFromBase58(params.B58_pubkey)
			if err != nil {
				return "", err
			}
			encrypted, err := crypto.Encrypt(public.Convert(), payload)
			if err != nil {
				return "", err
			}
			return hex.EncodeToString(encrypted), nil
		}
	default:
		{
		}
	}
	return "", fmt.Errorf("unsupported payload_method '%v'", params.Payload_method)
}

func (w *Wallet) PayloadDecrypt(_ context.Context, params *struct {
	Payload string
	Pwds    []string
}) (string, error) {
	payload, err := hex.DecodeString(params.Payload)
	if err != nil {
		return "", err
	}

	decrypted := []byte(nil)
	err = w.key.With(func(private *crypto.Key) error {
		decrypted, err = crypto.Decrypt(private, payload)
		return err
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(decrypted), nil
}

func (w *Wallet) sendRaw(transaction tx.CommonOperation) (txID string, err error) {
	raw := []byte(nil)

	err = w.key.With(func(private *crypto.Key) error {
		txID, raw, err = tx.Sign(transaction, private)
		return err
	})
	if err != nil {
		return "", err
	}

	result := false
	if err := w.client.Call("executeoperations", struct {
		RawOperations string
	}{
		RawOperations: hex.EncodeToString(raw),
	}, &result); err != nil {
		return "", err
	}
	if !result {
		return "", fmt.Errorf("failed to send an operation")
	}
	return txID, nil
}

func (w *Wallet) SendTo(_ context.Context, params *struct {
	Sender         uint32
	Target         uint32
	Amount         float64
	Fee            float64
	Payload        string
	Payload_method string
}) (*Operation, error) {
	account, err := w.getAccount(params.Sender)
	if err != nil {
		return nil, err
	}

	payload, err := hex.DecodeString(params.Payload)
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		dest, err := w.getAccount(params.Target)
		if err != nil {
			return nil, err
		}
		destEncPublic, err := hex.DecodeString(dest.EncPubkey)
		if err != nil {
			return nil, err
		}
		destPublic := crypto.Public{}
		err = destPublic.Deserialize(bytes.NewBuffer(destEncPublic))
		if err != nil {
			return nil, err
		}

		if params.Payload_method != "dest" {
			return nil, fmt.Errorf("unsupported payload methond")
		}
		payload, err = crypto.Encrypt(destPublic.Convert(), payload)
		if err != nil {
			return nil, err
		}
	}

	transaction := tx.Transfer{
		Source:      params.Sender,
		OperationId: account.NOperation + 1,
		Destination: params.Target,
		Amount:      uint64(params.Amount * 10000),
		Fee:         uint64(params.Fee * 10000),
		Payload:     payload,
	}

	txID, err := w.sendRaw(&transaction)
	if err != nil {
		return nil, err
	}

	return &Operation{
		Account:        transaction.GetAccount(),
		Amount:         float64(transaction.GetAmount()) / -10000,
		Block:          0,
		Dest_account:   transaction.GetDestAccount(),
		Fee:            float64(transaction.GetFee()) / 10000,
		Opblock:        0,
		Ophash:         txID,
		Optxt:          nil,
		Optype:         uint8(transaction.GetType()),
		Payload:        hex.EncodeToString(transaction.GetPayload()),
		Sender_account: transaction.GetAccount(),
		Time:           0,
	}, nil
}

func (w *Wallet) ChangeKey(_ context.Context, params *struct {
	Account        uint32
	New_enc_pubkey string
	New_b58_pubkey string
	Fee            float64
	Payload        string
	Payload_method string
}) (*Operation, error) {
	account, err := w.getAccount(params.Account)
	if err != nil {
		return nil, err
	}

	destPublic, err := crypto.PublicFromBase58(params.New_b58_pubkey)
	if err != nil {
		return nil, err
	}
	destRaw := bytes.NewBuffer(nil)
	if err := destPublic.Serialize(destRaw); err != nil {
		return nil, err
	}

	payload, err := hex.DecodeString(params.Payload)
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		if params.Payload_method != "dest" {
			return nil, fmt.Errorf("unsupported payload methond")
		}
		payload, err = crypto.Encrypt(destPublic.Convert(), payload)
		if err != nil {
			return nil, err
		}
	}

	if params.Fee < 0 {
		return nil, fmt.Errorf("invalid fee")
	}
	if account.Balance < params.Fee {
		return nil, fmt.Errorf("insufficient balance")
	}

	transaction := tx.ChangeKey{
		Source:       params.Account,
		OperationId:  account.NOperation + 1,
		Fee:          uint64(params.Fee * 10000),
		Payload:      payload,
		NewPublickey: destRaw.Bytes(),
	}

	txID, err := w.sendRaw(&transaction)
	if err != nil {
		return nil, err
	}

	return &Operation{
		Account:        transaction.GetAccount(),
		Amount:         float64(transaction.GetAmount()) / -10000,
		Block:          0,
		Dest_account:   transaction.GetDestAccount(),
		Fee:            float64(transaction.GetFee()) / 10000,
		Opblock:        0,
		Ophash:         txID,
		Optxt:          nil,
		Optype:         uint8(transaction.GetType()),
		Payload:        hex.EncodeToString(transaction.GetPayload()),
		Sender_account: transaction.GetAccount(),
		Time:           0,
	}, nil
}
