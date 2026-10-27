package aap2_client

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"strings"

	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"

	"github.com/google/uuid"
)

// AAP2BaseClient represents a context manager for connecting
// to µD3TN's AAP 2.0 socket.
//
// Note:
//
//	AAP2BaseClient provides the core context fields and methods
//	that should be reused by embedding this struct into other
//	specific communication types (e.g. AAP2SyncClient, AAP2AsyncClient)
type AAP2BaseClient struct {
	address    string
	nodeEid    string
	agentId    string
	connection net.Conn
}

func (a *AAP2BaseClient) init(address string) {
	a.address = address
	a.nodeEid = ""
	a.agentId = ""
}

// Eid returns the AAP2 clients own EID including agent ID.
func (a *AAP2BaseClient) Eid() string {
	if a.nodeEid[0:3] == "ipn" {
		prefix := strings.Split(a.nodeEid, ".")[0]
		return fmt.Sprintf("%v.%v", prefix, a.agentId)
	} else {
		return fmt.Sprintf("%v%v", a.nodeEid, a.agentId)
	}
}

// EidPrefix returns the EID prefix.
func (a *AAP2BaseClient) EidPrefix() string {
	if a.nodeEid[0:3] == "ipn" {
		prefix := strings.Split(a.nodeEid, ".")[0]
		return prefix
	} else {
		return a.nodeEid
	}
}

// IsIpnEid returns true if the connected node uses the `ipn` EID scheme.
func (a AAP2BaseClient) IsIpnEid() bool {
	return a.nodeEid[0:3] == "ipn"
}

// ResponseStatusName returns the name for the specified AAP2 ResponseStatus code.
//
// Args:
//
//	status: The integer ResponseStatus code returned by µD3TN.
//
// Return:
//
//	A string representation of status or "<unknown>" for values not
//	part of the ResponseStatus_name map.
func (a *AAP2BaseClient) ResponseStatusName(status int32) string {
	if n, ok := aap2.ResponseStatus_name[status]; ok {
		return n
	}
	return "<unknown>"
}

// generateAgentId is used to generate a random agent ID if the user did not supply one.
func (a *AAP2BaseClient) generateAgentId() string {
	if a.nodeEid == "" {
		return ""
	}

	if a.IsIpnEid() {
		return fmt.Sprint((rand.Uint32N(math.MaxUint32-1) + 1))
	} else if a.nodeEid[0:3] == "dtn" {
		return uuid.NewString()
	}
	return ""
}

// checkFirstByte checks the first byte received from the server,
// which must be 0x2f.
func (a *AAP2BaseClient) checkFirstByte(data []byte) error {
	if len(data) == 0 {
		return errors.New("connection broke during handshake")
	}
	if data[0] != 0x2f {
		return errors.New("did not receive AAP 2.0 magic number 0x2f")
	}
	return nil
}

// processWelcome decodes and stores properties received in the WELCOME message.
func (a *AAP2BaseClient) processWelcome(msgWelcome *aap2.AAPMessage) (*aap2.Welcome, error) {
	if _, ok := msgWelcome.GetMsg().(*aap2.AAPMessage_Welcome); !ok {
		return nil, fmt.Errorf("expected WELCOME message, got: %v, %v", msgWelcome.GetMsg(), msgWelcome)
	}
	a.nodeEid = msgWelcome.GetWelcome().GetNodeId()
	slog.Debug(fmt.Sprintf("WELCOME message received! ~ EID = %v", a.nodeEid))
	return msgWelcome.GetWelcome(), nil
}

// prepareConfigMsg creates an AAPMessage for the CONFIGURE operation.
func (a *AAP2BaseClient) prepareConfigMsg(agentId string, subscribe bool, secret string, authType aap2.AuthType, keepaliveSeconds uint32) (aap2.AAPMessage_Config, string) {
	if agentId == "" {
		a.agentId = a.generateAgentId()
	} else {
		a.agentId = agentId
	}

	var eid string
	if a.IsIpnEid() {
		eid = fmt.Sprintf("%v.%v", a.EidPrefix(), a.agentId)
	} else {
		eid = fmt.Sprintf("%v%v", a.EidPrefix(), a.agentId)
	}

	if secret == "" {
		secret = uuid.NewString()
	}

	configMsg := aap2.ConnectionConfig{
		AuthType:         authType,
		IsSubscriber:     subscribe,
		EndpointId:       eid,
		Secret:           secret,
		KeepaliveSeconds: keepaliveSeconds,
	}

	slog.Debug(fmt.Sprintf("Sending CONFIGURE message for '%v'...", agentId))
	return aap2.AAPMessage_Config{Config: &configMsg}, secret
}

// checkSuccess checks that the provided AAPResponse reports success.
func (a *AAP2BaseClient) checkSuccess(response *aap2.AAPResponse) error {
	if response.ResponseStatus != aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS {
		return fmt.Errorf("server returned invalid response status: %v", response.ResponseStatus)
	}
	return nil
}
