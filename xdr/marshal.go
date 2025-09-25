package xdr

import (
	"bytes"
)

func Marshal(src ...interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	encoder := NewEncoder(buf)

	for _, v := range src {
		if err := encoder.Encode(v); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func Unmarshal(data []byte, dst ...interface{}) ([]byte, error) {
	consumed := 0

	for _, v := range dst {
		if n, err := NewDecoder(bytes.NewReader(data[consumed:])).Read(v); err != nil {
			return data[consumed:], err
		} else {
			consumed += n
		}
	}

	return data[consumed:], nil
}
