package tx

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/pasl-project/pasl/crypto"
	"github.com/pasl-project/pasl/utils"
)

func TestSerialize(t *testing.T) {
	serializedTx, _ := hex.DecodeString("020000008fd003000100000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669014600ca02200089927599939edd01c65628e7e25f7a1ff511c9806d75dbf2917131c4217814dd20006cf4bc42292ed111c111d17d1a7b37d36f077340b60e918da2aa424ce2777d8b2000e90d7f237b1ba873103e11eb4f0002d35ee43d3b7c295ec7fe5d0393abe236372000e0491191fc9567fceb4c6c91d4d6e95edecff23f2a45cb60b5bddd613c2d348a")
	var tx TxSerialized
	if err := utils.Deserialize(&tx, bytes.NewBuffer(serializedTx)); err != nil {
		t.Fatal(err)
	}

	buf := bytes.NewBuffer(nil)
	if err := tx.Serialize(buf); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(serializedTx, buf.Bytes()) {
		t.FailNow()
	}

	txMetadata := GetMetadata(tx, 0, 0, 0)
	if txType(txMetadata.Type) != tx.GetType() {
		t.FailNow()
	}
}

func TestDeserializeOperationsNetwork(t *testing.T) {
	serialized := hex.NewDecoder(bytes.NewBuffer([]byte("04000000028f2210000100000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669014600ca0220005966f012c91daab2370e3eb25c1b6769e69e60569b01acc334566cd227b054cd2000ee1641202de36fab5abbac73fc36bad234ae8327be5f2782a06d32678254216220007ef4598bddabf679336df0042925a629d9254d8f12055d44f9437611def0305d2000769eabbe701a2a4ada5a720b6715e36c9f93b8972d1b86cf6df41df45823410d01de201000010000008f22100084de01000000000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669012000dcd09dae6a2376156564c87dc319c770b54af0fafcc7a135e37ce76f71af284a20008c38b90412a1f7262ff4dd5e7e74d31721e198025623f0eeff809405654c460f028e2210000100000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669014600ca0220005966f012c91daab2370e3eb25c1b6769e69e60569b01acc334566cd227b054cd2000ee1641202de36fab5abbac73fc36bad234ae8327be5f2782a06d326782542162200082725543b47872f77731dcaa6c3e046150c025732451c02b605bb1a2f66cc21c20007acadde720a066f6a7b27655bbdec93f43944a7dd027a183fa57d8e40e7a6d8e01de201000020000008e2210007c6000000000000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669012000781ae1761628fa65f44f7a04617ccf69e6421b4e708d380db1a5eb8f86b68f0220008e928d4e6e31f7d3675655ceb669d9dd2ad99214d666e9734eb229b625df2417")))

	var txes OperationsNetwork
	if err := utils.Deserialize(&txes, serialized); err != nil {
		t.Fatal(err)
	}
	if len(txes.Operations) != 4 {
		t.FailNow()
	}
	for each := range txes.Operations {
		if txes.Operations[each] == nil {
			t.FailNow()
		}
	}
}

func TestSignature(t *testing.T) {
	raw := []byte{}
	key, err := crypto.NewKeyByType(crypto.NIDsecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	{
		serializedTx, _ := hex.DecodeString("020000008fd003000100000000000000000000000000ca022000666293eb108763de780fd6ee5f2d8f92a9c69fc3e36b5a40a9e8d25523f619c7200097fc795b55d50a41dc8abd099adf96152a2f07a1c35480dd4512abc4e42669014600ca02200089927599939edd01c65628e7e25f7a1ff511c9806d75dbf2917131c4217814dd20006cf4bc42292ed111c111d17d1a7b37d36f077340b60e918da2aa424ce2777d8b2000e90d7f237b1ba873103e11eb4f0002d35ee43d3b7c295ec7fe5d0393abe236372000e0491191fc9567fceb4c6c91d4d6e95edecff23f2a45cb60b5bddd613c2d348a")
		var tx TxSerialized
		if err := utils.Deserialize(&tx, bytes.NewBuffer(serializedTx)); err != nil {
			t.Fatal(err)
		}
		if err := ValidateSignature(&tx); err != nil {
			t.Fatal(err)
		}
		_, raw, err = Sign(&tx, key)
		if err != nil {
			t.Fatal(err)
		}
	}

	var operations OperationsNetwork
	if err := utils.Deserialize(&operations, bytes.NewBuffer(raw)); err != nil {
		t.Fatal(err)
	}
	tx := operations.Operations[0]
	_, _, newPublic := tx.getSourceInfo()
	if !newPublic.Equal(key.Public) {
		t.FailNow()
	}
	if err := ValidateSignature(tx); err != nil {
		t.Fatal(err)
	}
}
