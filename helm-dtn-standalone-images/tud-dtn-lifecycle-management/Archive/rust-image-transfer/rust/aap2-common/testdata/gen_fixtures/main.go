// Throwaway program to capture AAP2 wire-format fixtures from the
// existing Go client's own encoding, so the Rust client's framing can be
// tested against real Go-produced bytes rather than just re-implementing
// the spec and hoping both sides agree.
package main

import (
	"encoding/hex"
	"fmt"

	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func lengthDelimited(msg proto.Message) []byte {
	bin, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	// Mirror aap2_sync_client.go's Send(): varint length prefix + bytes.
	var varintBuf [10]byte
	n := putUvarint(varintBuf[:], uint64(len(bin)))
	return append(varintBuf[:n], bin...)
}

// putUvarint is copied from encoding/binary to avoid importing it just for
// this one call in a throwaway tool - actually just use encoding/binary.
func putUvarint(buf []byte, x uint64) int {
	i := 0
	for x >= 0x80 {
		buf[i] = byte(x) | 0x80
		x >>= 7
		i++
	}
	buf[i] = byte(x)
	return i + 1
}

func main() {
	configMsg := &aap2.AAPMessage{
		Msg: &aap2.AAPMessage_Config{
			Config: &aap2.ConnectionConfig{
				IsSubscriber:     true,
				AuthType:         aap2.AuthType_AUTH_TYPE_DEFAULT,
				Secret:           "fixture-secret-123",
				EndpointId:       "dtn://earth.dtn/fixture-agent",
				KeepaliveSeconds: 0,
			},
		},
	}
	fmt.Printf("CONFIG_MSG=%s\n", hex.EncodeToString(lengthDelimited(configMsg)))

	responseMsg := &aap2.AAPResponse{
		ResponseStatus: aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS,
	}
	fmt.Printf("RESPONSE_SUCCESS=%s\n", hex.EncodeToString(lengthDelimited(responseMsg)))

	aduMsg := &aap2.AAPMessage{
		Msg: &aap2.AAPMessage_Adu{
			Adu: &aap2.BundleADU{
				DstEid:        "dtn://space.dtn/sink",
				PayloadLength: 12345,
			},
		},
	}
	fmt.Printf("ADU_MSG=%s\n", hex.EncodeToString(lengthDelimited(aduMsg)))

	welcomeMsg := &aap2.AAPMessage{
		Msg: &aap2.AAPMessage_Welcome{
			Welcome: &aap2.Welcome{
				NodeId: "dtn://earth.dtn/",
			},
		},
	}
	fmt.Printf("WELCOME_MSG=%s\n", hex.EncodeToString(lengthDelimited(welcomeMsg)))
}
