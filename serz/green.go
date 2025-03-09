package art

import (
	"github.com/glycerine/greenpack/msgp"
)

//go:generate greenpack

type Green interface {
	//msgp.Encodable // maybe comment out later.
	msgp.Encodable
	msgp.Decodable
	msgp.Marshaler
	msgp.Unmarshaler
}

type GreenWrapp struct {
	Inside any `zid:"0"`
}
