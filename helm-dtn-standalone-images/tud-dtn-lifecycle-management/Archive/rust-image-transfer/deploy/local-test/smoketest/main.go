// Command smoketest is a throwaway program (TASKS.md Phase 1) that proves
// the Docker Compose two-node µD3TN harness works using the EXISTING Go
// aap2_client package, before any Rust code exists. This way, if something
// breaks once the Rust AAP2 client shows up in Phase 3, we already know
// whether the bug is in the harness or in the new client.
//
// Not part of the helm-dtn plugin proper - lives under deploy/local-test/
// and is not built/installed by anything else.
//
// Usage:
//
//	smoketest send    --tcp 127.0.0.1:14242 --dest dtn://space.dtn/sink --payload-size 65536
//	smoketest receive --tcp 127.0.0.1:24242 --agent sink
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/darksol-cloud/helm-dtn/pkg/aap2_client"
	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: smoketest <send|receive> [flags]")
	}
	switch os.Args[1] {
	case "send":
		runSend(os.Args[2:])
	case "receive":
		runReceive(os.Args[2:])
	default:
		log.Fatalf("unknown subcommand %q (want send|receive)", os.Args[1])
	}
}

func runSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	tcp := fs.String("tcp", "127.0.0.1:14242", "AAP2 TCP address of the sending node")
	dest := fs.String("dest", "dtn://space.dtn/sink", "destination EID")
	payloadSize := fs.Int("payload-size", 65536, "number of random bytes to send")
	fs.Parse(args)

	payload := make([]byte, *payloadSize)
	if _, err := rand.Read(payload); err != nil {
		log.Fatalf("generating random payload: %v", err)
	}
	sum := sha256.Sum256(payload)
	fmt.Printf("send: generated %d random bytes, sha256=%s\n", len(payload), hex.EncodeToString(sum[:]))

	client := aap2_client.NewAAP2TCPClient(*tcp)
	if err := client.Connect(); err != nil {
		log.Fatalf("connect to %s: %v", *tcp, err)
	}
	defer client.Disconnect()

	if _, err := client.Configure(aap2_client.WithAgentId("smoketest-sender")); err != nil {
		log.Fatalf("configure: %v", err)
	}
	fmt.Println("send: configured, sending ADU...")

	if err := client.SendAdu(&aap2.BundleADU{
		DstEid:        *dest,
		PayloadLength: uint64(len(payload)),
	}, payload); err != nil {
		log.Fatalf("SendAdu: %v", err)
	}

	resp, err := client.ReceiveResponse()
	if err != nil {
		log.Fatalf("ReceiveResponse: %v", err)
	}
	if resp.ResponseStatus != aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS {
		log.Fatalf("send rejected: status=%s", client.ResponseStatusName(int32(resp.ResponseStatus)))
	}
	fmt.Printf("send: OK, µD3TN accepted the bundle (status=%s)\n", client.ResponseStatusName(int32(resp.ResponseStatus)))
	fmt.Printf("SMOKETEST_SHA256=%s\n", hex.EncodeToString(sum[:]))
}

func runReceive(args []string) {
	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	tcp := fs.String("tcp", "127.0.0.1:24242", "AAP2 TCP address of the receiving node")
	agent := fs.String("agent", "sink", "agent ID to register for receiving")
	expectSha256 := fs.String("expect-sha256", "", "if set, verify the received payload's sha256 matches and exit non-zero on mismatch")
	fs.Parse(args)

	client := aap2_client.NewAAP2TCPClient(*tcp)
	if err := client.Connect(); err != nil {
		log.Fatalf("connect to %s: %v", *tcp, err)
	}
	defer client.Disconnect()

	if _, err := client.Configure(
		aap2_client.WithAgentId(*agent),
		aap2_client.WithSubscribe(true),
	); err != nil {
		log.Fatalf("configure: %v", err)
	}
	fmt.Printf("receive: registered agent %q, waiting for one ADU...\n", *agent)

	msg, err := client.ReceiveMsg()
	if err != nil {
		log.Fatalf("ReceiveMsg: %v", err)
	}
	if err := client.SendResponseStatus(aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS); err != nil {
		log.Fatalf("SendResponseStatus: %v", err)
	}
	aduMsg, ok := msg.GetMsg().(*aap2.AAPMessage_Adu)
	if !ok {
		log.Fatalf("expected ADU message, got: %T", msg.GetMsg())
	}
	_, payload, err := client.ReceiveAdu(aduMsg.Adu)
	if err != nil {
		log.Fatalf("ReceiveAdu: %v", err)
	}

	sum := sha256.Sum256(payload)
	gotSha := hex.EncodeToString(sum[:])
	fmt.Printf("receive: got %d bytes from %s, sha256=%s\n", len(payload), aduMsg.Adu.SrcEid, gotSha)
	fmt.Printf("SMOKETEST_SHA256=%s\n", gotSha)

	if *expectSha256 != "" {
		if gotSha != *expectSha256 {
			log.Fatalf("CHECKSUM MISMATCH: expected %s, got %s", *expectSha256, gotSha)
		}
		fmt.Println("receive: checksum MATCHES expected value - byte-for-byte integrity confirmed")
	}
}
