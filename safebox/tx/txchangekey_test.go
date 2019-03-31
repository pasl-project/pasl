package tx

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/pasl-project/pasl/utils"
)

func TestTxSignBuffer(t *testing.T) {
	serializedTx, _ := hex.DecodeString("020000008fd003000100000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669014600ca02200089927599939edd01c65628e7e25f7a1ff511c9806d75dbf2917131c4217814dd20006cf4bc42292ed111c111d17d1a7b37d36f077340b60e918da2aa424ce2777d8b2000e90d7f237b1ba873103e11eb4f0002d35ee43d3b7c295ec7fe5d0393abe236372000e0491191fc9567fceb4c6c91d4d6e95edecff23f2a45cb60b5bddd613c2d348a")
	var tx Tx
	if utils.Deserialize(&tx, bytes.NewBuffer(serializedTx)) != nil {
		t.Fatal()
	}

	valid, _ := hex.DecodeString("3532454142363930464233383130444234433437")
	txRipemd160Hash := tx.GetRipemd16Hash()
	if !bytes.Equal(valid, txRipemd160Hash) {
		t.Fatalf("%x != %x", valid, txRipemd160Hash)
	}

	if tx.GetAmount() != 0 {
		t.FailNow()
	}
	if tx.GetDestAccount() != tx.GetAccount() {
		t.FailNow()
	}
	if tx.GetFee() != 0 {
		t.FailNow()
	}
	if !bytes.Equal(tx.GetPayload(), []byte{}) {
		t.FailNow()
	}
}
