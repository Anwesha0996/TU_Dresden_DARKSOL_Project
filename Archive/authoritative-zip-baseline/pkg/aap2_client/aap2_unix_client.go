package aap2_client

import (
	"fmt"
	"net"

	"log/slog"
)

// AAP2UnixClient implements a synchronous client for
// connecting to µD3TN's AAP 2.0 Unix socket.
type AAP2UnixClient struct {
	AAP2SyncClient
}

func NewAAP2UnixClient(address string) *AAP2UnixClient {
	client := &AAP2UnixClient{
		AAP2SyncClient: AAP2SyncClient{
			AAP2BaseClient: AAP2BaseClient{},
		},
	}
	client.init(address)
	return client
}

func (a *AAP2UnixClient) Connect() error {
	raddr, err := net.ResolveUnixAddr("unix", a.address)

	if err != nil {
		return err
	}
	conn, err := net.DialUnix("unix", nil, raddr)
	if err != nil {
		return err
	}

	a.connection = conn
	slog.Info(fmt.Sprintf("Successfully connected to: %s", a.address))
	a.welcome()
	return nil
}

func (a *AAP2UnixClient) Disconnect() {
	if err := a.connection.Close(); err != nil {
		slog.Error(err.Error())
	}
}
