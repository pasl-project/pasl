package wallet

type SecureBuffer struct {
	Data      []byte
	Encrypted bool
}

func NewSecureBuffer(data []byte) SecureBuffer {
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return SecureBuffer{
		Data:      dataCopy,
		Encrypted: false,
	}
}

func (s *SecureBuffer) With(fn func(data []byte)) {
	fn(s.Data)
}
