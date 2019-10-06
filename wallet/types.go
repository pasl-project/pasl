package wallet

type Public struct {
	TypeID    uint16 `json:"ec_nid"`
	X         string `json:"x"`
	Y         string `json:"y"`
	EncPubkey string `json:"enc_pubkey"`
	B58Pubkey string `json:"b58_pubkey"`
}

type Account struct {
	*Public
	Name   string
	CanUse bool `json:"can_use"`
}

type Operation struct {
	Account        uint32  `json:"account"`
	Amount         float64 `json:"amount"`
	Block          uint32  `json:"block"`
	Dest_account   uint32  `json:"dest_account"`
	Fee            float64 `json:"fee"`
	Opblock        uint32  `json:"opblock"`
	Ophash         string  `json:"ophash"`
	Optxt          *string `json:"optxt"`
	Optype         uint8   `json:"optype"`
	Payload        string  `json:"payload"`
	Sender_account uint32  `json:"sender_account"`
	Time           uint32  `json:"time"`
}
