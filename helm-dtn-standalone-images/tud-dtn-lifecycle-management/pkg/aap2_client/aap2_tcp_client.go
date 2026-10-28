package aap2_client

import (
	"fmt"
	"log/slog"
	"net"
)

type AAP2TCPClient struct {
	AAP2SyncClient
}

func NewAAP2TCPClient(address string) *AAP2TCPClient {
	client := &AAP2TCPClient{
		AAP2SyncClient: AAP2SyncClient{
			AAP2BaseClient: AAP2BaseClient{},
		},
	}
	client.init(address)
	return client
}

func (a *AAP2TCPClient) Connect() error {
	conn, err := net.Dial("tcp", a.address)

	if err != nil {
		return err
	}

	a.connection = conn
	slog.Info(fmt.Sprintf("Successfully connected to: %s", a.address))
	a.welcome()
	return nil
}

func (a *AAP2TCPClient) Disconnect() {
	if err := a.connection.Close(); err != nil {
		slog.Error(err.Error())
	}
}
