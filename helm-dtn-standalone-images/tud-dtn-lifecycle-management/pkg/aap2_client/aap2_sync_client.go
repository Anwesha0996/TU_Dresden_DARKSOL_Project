package aap2_client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// AAP2SyncClient represents AAP clients implementing
// synchronous (blocking) operations.
// It embeds AAP2BaseClient to inherit fundamental fields
// and methods.
type AAP2SyncClient struct {
	AAP2BaseClient
}

// welcome receives the AAP welcome message and stores the node EID
// of the µD3TN instance in the `nodeEid` field of the AAP2BaseClient.
func (a *AAP2SyncClient) welcome() (*aap2.Welcome, error) {
	buffer := make([]byte, 1)

	if _, err := a.connection.Read(buffer); err != nil {
		return nil, fmt.Errorf("welcome(): failed to read first byte: %w", err)
	}

	if err := a.checkFirstByte(buffer); err != nil {
		return nil, fmt.Errorf("welcome(): check of first byte failed: %w", err)
	}
	msg, err := a.ReceiveMsg()
	if err != nil {
		return nil, fmt.Errorf("welcome(): failed to receive message: %w", err)
	}

	welcome, err := a.processWelcome(msg)

	if err != nil {
		return nil, fmt.Errorf("welcome(): failed to process WELCOME message: %w", err)
	}

	return welcome, nil
}

// Configure attempts to configure the conncetion as specified.
// Returns the secret that can be used for registering additional connections.
func (a *AAP2SyncClient) Configure(opts ...AAP2ConfigurationOption) (string, error) {
	configuration := &AAP2ConfigurationOptions{
		AgentId:          "",
		Subscribe:        false,
		Secret:           "",
		AuthType:         aap2.AuthType_AUTH_TYPE_DEFAULT,
		KeepaliveSeconds: 0,
	}

	for _, opt := range opts {
		opt(configuration)
	}
	configMsg, secret := a.prepareConfigMsg(configuration.AgentId, configuration.Subscribe, configuration.Secret, configuration.AuthType, configuration.KeepaliveSeconds)
	msg := &aap2.AAPMessage{
		Msg: &configMsg,
	}
	a.Send(msg)
	response, err := a.ReceiveResponse()
	if err != nil {
		return "", errors.New("configure(): failed to receive response")
	}
	if err := a.checkSuccess(response); err != nil {
		return "", fmt.Errorf("configure(): %w", err)
	}
	return secret, nil
}

// receiveAll receives exactly `count` bytes from the socket.
func (a *AAP2SyncClient) receiveAll(count int) ([]byte, error) {
	buf := make([]byte, count)

	if _, err := io.ReadFull(a.connection, buf); err != nil {
		return nil, fmt.Errorf("receiveAll(): failed to read data: %w", err)
	}
	return buf, nil
}

// receiveDelimited receives and returns a byte slice for which the
// length is indicated in a preceding Protobuf varint.
func (a *AAP2SyncClient) receiveDelimited() ([]byte, error) {
	const PROTOBUF_VARINT_MAX_BYTES = 10
	var result bytes.Buffer
	buf := make([]byte, 1)
	c := 0
	for c < PROTOBUF_VARINT_MAX_BYTES {

		if _, err := a.connection.Read(buf); err != nil {
			return nil, fmt.Errorf("receiveDelimited(): failed to read from connection: %w", err)
		}
		result.Write(buf)

		if buf[0]&0x80 == 0 {
			break
		}
		c += 1

	}
	if c >= PROTOBUF_VARINT_MAX_BYTES {
		return nil, errors.New("receiveDelimited(): invalid varint received")
	}
	dataLen, _ := binary.Uvarint(result.Bytes())
	return a.receiveAll(int(dataLen))

}

// ReceiveMsg receives and returns the next `AAPMessage`.
func (a *AAP2SyncClient) ReceiveMsg() (*aap2.AAPMessage, error) {
	msgBin, err := a.receiveDelimited()
	if err != nil {
		return nil, err
	}
	msg := &aap2.AAPMessage{}

	err = proto.Unmarshal(msgBin, msg)
	if err != nil {
		return nil, fmt.Errorf("ReceiveMsg(): failed to parse message: %w", err)
	}

	return msg, nil

}

// ReceiveResponse receives and returns the next `AAPResponse`
func (a *AAP2SyncClient) ReceiveResponse() (*aap2.AAPResponse, error) {
	msgBin, err := a.receiveDelimited()
	if err != nil {
		return nil, fmt.Errorf("ReceiveResponse(): %w", err)
	}
	resp := &aap2.AAPResponse{}
	err = proto.Unmarshal(msgBin, resp)
	if err != nil {
		return nil, fmt.Errorf("ReceiveResponse(): failed to parse message: %w", err)
	}
	return resp, nil
}

// ReceiveAdu receives a bundle ADU (Protobuf with extra payload data).
// If the `BundleADU` message was already received, it must be passed using this argument.
// Otherwise, the function will call `receive_msg` to receive the next message,
// which must be of type `BundleADU`.
func (a *AAP2SyncClient) ReceiveAdu(aduMsg *aap2.BundleADU) (*aap2.BundleADU, []byte, error) {
	if aduMsg == nil {
		msg, err := a.ReceiveMsg()
		if err != nil {
			return nil, nil, err
		}
		if _, ok := msg.GetMsg().(*aap2.AAPMessage_Adu); !ok {
			return nil, nil, fmt.Errorf("ReceiveAdu(): expected ADU field, got: %v, %v", msg.GetMsg(), msg)
		}
		aduMsg = msg.GetMsg().(*aap2.AAPMessage_Adu).Adu
	}
	bundleLen := aduMsg.PayloadLength
	bundleData := make([]byte, bundleLen)

	if _, err := io.ReadFull(a.connection, bundleData); err != nil {
		return nil, nil, fmt.Errorf("ReceiveAdu(): error reading bundle data: ,%v", err)
	}
	return aduMsg, bundleData, nil
}

// ReceiveAduFile streams an ADU payload to disk instead of allocating the
// complete container image in memory.
func (a *AAP2SyncClient) ReceiveAduFile(aduMsg *aap2.BundleADU, filePath string) (*aap2.BundleADU, error) {
	if aduMsg == nil {
		msg, err := a.ReceiveMsg()
		if err != nil {
			return nil, err
		}
		aduMsg = msg.GetAdu()
		if aduMsg == nil {
			return nil, fmt.Errorf("ReceiveAduFile(): expected ADU")
		}
	}
	out, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.CopyN(out, a.connection, int64(aduMsg.PayloadLength))
	closeErr := out.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("ReceiveAduFile(): %w", copyErr)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return aduMsg, nil
}

// Send serializes and sends the provided Protobuf message including header.
func (a *AAP2SyncClient) Send(msg proto.Message) error {
	msgBin, err := proto.Marshal(msg)
	fmt.Println(msg)
	if err != nil {
		return fmt.Errorf("Send(): failed to marshal proto.Message: %w", err)
	}

	msgLen := uint64(len(msgBin))

	var varintBuf [binary.MaxVarintLen64]byte

	n := binary.PutUvarint(varintBuf[:], msgLen)
	delimitedMsg := append(varintBuf[:n], msgBin...)
	if _, err := a.connection.Write(delimitedMsg); err != nil {
		return fmt.Errorf("Send(): failed to send message: %w", err)
	}

	return nil
}

// SendAdu sends a bundle ADU (Protobuf with extra payload data).
func (a *AAP2SyncClient) SendAdu(aduMsg *aap2.BundleADU, bundleData []byte) error {
	if aduMsg.PayloadLength != uint64(len(bundleData)) {
		return fmt.Errorf("SendAdu(): payload length encoded in aduMsg does not match actual payload length: %v != %v", aduMsg.PayloadLength, len(bundleData))
	}
	aapMsg := &aap2.AAPMessage{Msg: &aap2.AAPMessage_Adu{Adu: aduMsg}}
	if err := a.Send(aapMsg); err != nil {
		return err
	}

	buf := bytes.NewBuffer(bundleData)

	if _, err := io.Copy(a.connection, buf); err != nil {
		return fmt.Errorf("SendAdu(): failed to write bundleData to connection: %w", err)
	}
	return nil
}

// SendFile sends a bundle ADU (Protobuf with extra payload data).
func (a *AAP2SyncClient) SendFile(aduMsg *aap2.BundleADU, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if uint64(info.Size()) != aduMsg.PayloadLength {
		return fmt.Errorf("SendFile(): payload length encoded in aduMsg does not match file size: %d != %d", aduMsg.PayloadLength, info.Size())
	}
	aapMsg := &aap2.AAPMessage{Msg: &aap2.AAPMessage_Adu{Adu: aduMsg}}
	if err := a.Send(aapMsg); err != nil {
		return err
	}
	if _, err := io.Copy(a.connection, file); err != nil {
		return fmt.Errorf("SendFile(): failed to write payload: %w", err)
	}
	return nil
}

// SendResponseStatus sends an AAPResponse with the specified status code.
func (a *AAP2SyncClient) SendResponseStatus(status aap2.ResponseStatus) error {
	response := &aap2.AAPResponse{ResponseStatus: status}
	if err := a.Send(response); err != nil {
		return err
	}
	return nil
}
