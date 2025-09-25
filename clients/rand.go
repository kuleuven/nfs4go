package clients

import (
	"crypto/rand"
	"math"
	"math/big"
)

func randUint64() uint64 {
	val, err := rand.Int(rand.Reader, big.NewInt(int64(math.MaxInt64)))
	if err != nil {
		panic(err)
	}

	return val.Uint64()
}
