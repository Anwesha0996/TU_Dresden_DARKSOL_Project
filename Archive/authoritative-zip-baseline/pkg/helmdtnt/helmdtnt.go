package helmdtnt

import (
	"log"
	"os"

	"github.com/darksol-cloud/helm-dtn/pkg/aap2_client"
	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
)

func SendChart(ud3tnSocket, archivePath, tcp, destEID string) error {
	log.Println("helmdtnt: Trying to send archive to remote cluster...")
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	return send(payload, ud3tnSocket, tcp, destEID)
}

func SendUninstall(release, ud3tnSocket, tcp, destEID string) error {
	log.Printf("helmdtnt: Trying to send 'uninstall' for %v to remote cluster...\n", release)
	payload := []byte(release)
	return send(payload, ud3tnSocket, tcp, destEID)

}

func send(payload []byte, ud3tnSocket, tcp, destEID string) error {
	// FIXME: Ugly code duplication, read up on Go generics and try to refactor into a single function for sending data.
	if ud3tnSocket != "" {
		sendClient := aap2_client.NewAAP2UnixClient(ud3tnSocket)
		if err := sendClient.Connect(); err != nil {
			return err
		}
		defer sendClient.Disconnect()

		if _, err := sendClient.Configure(aap2_client.WithAgentId("helmd-dtn-agent")); err != nil {
			return err
		}

		sendClient.SendAdu(&aap2.BundleADU{
			DstEid:        destEID,
			PayloadLength: uint64(len(payload)),
		}, payload)

		_, err := sendClient.ReceiveResponse()
		return err
	} else {
		sendClient := aap2_client.NewAAP2TCPClient(tcp)
		if err := sendClient.Connect(); err != nil {
			return err
		}
		defer sendClient.Disconnect()

		if _, err := sendClient.Configure(aap2_client.WithAgentId("helmd-dtn-agent")); err != nil {
			return err
		}

		sendClient.SendAdu(&aap2.BundleADU{
			DstEid:        destEID,
			PayloadLength: uint64(len(payload)),
		}, payload)

		_, err := sendClient.ReceiveResponse()

		return err
	}
}
