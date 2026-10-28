package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darksol-cloud/helm-dtn/pkg/aap2_client"
	"github.com/darksol-cloud/helm-dtn/pkg/imagetransfer"
	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
)

func main() {
	tcp := flag.String("tcp", "", "remote micro-D3TN AAP2 address")
	agent := flag.String("agent-id", "image-transfer", "AAP2 subscriber agent ID")
	secret := flag.String("secret", "", "AAP2 secret, if configured")
	registry := flag.String("registry", "", "destination OCI registry and optional repository prefix")
	workDir := flag.String("work-dir", "/tmp/go-image-receiver", "persistent receiver working directory")
	insecure := flag.Bool("insecure-registry", false, "allow a plain HTTP registry (testing only)")
	flag.Parse()
	if *tcp == "" || *registry == "" {
		flag.Usage()
		os.Exit(2)
	}
	creds := os.Getenv("REGISTRY_CREDS")
	if creds == "" {
		creds = os.Getenv("HARBOR_CREDS") // backwards compatibility
	}
	user, pass := "", ""
	if creds != "" {
		var ok bool
		user, pass, ok = strings.Cut(creds, ":")
		if !ok {
			log.Fatal("REGISTRY_CREDS must be user:password")
		}
	}
	if err := os.MkdirAll(*workDir, 0700); err != nil {
		log.Fatal(err)
	}
	for {
		if err := receive(*tcp, *agent, *secret, *registry, user, pass, *workDir, *insecure); err != nil {
			log.Printf("AAP2 connection failed: %v; reconnecting", err)
			time.Sleep(5 * time.Second)
		}
	}
}

func receive(tcp, agent, secret, registry, user, pass, workDir string, insecure bool) error {
	client := aap2_client.NewAAP2TCPClient(tcp)
	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Disconnect()
	opts := []aap2_client.AAP2ConfigurationOption{aap2_client.WithAgentId(agent), aap2_client.WithSubscribe(true)}
	if secret != "" {
		opts = append(opts, aap2_client.WithSecret(secret))
	}
	if _, err := client.Configure(opts...); err != nil {
		return err
	}
	log.Printf("registered dtn image endpoint %q", agent)
	for {
		msg, err := client.ReceiveMsg()
		if err != nil {
			return err
		}
		adu := msg.GetAdu()
		if adu == nil {
			continue
		}
		if err := client.SendResponseStatus(aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS); err != nil {
			return err
		}
		payload := filepath.Join(workDir, fmt.Sprintf("payload-%d", time.Now().UnixNano()))
		if _, err := client.ReceiveAduFile(adu, payload); err != nil {
			os.Remove(payload)
			return err
		}
		if err := handle(payload, registry, user, pass, workDir, insecure); err != nil {
			log.Printf("transfer rejected: %v", err)
		}
		os.Remove(payload)
	}
}

func handle(payload, registry, user, pass, workDir string, insecure bool) error {
	archive := payload + ".oci.tar"
	defer os.Remove(archive)
	h, err := imagetransfer.DecodePayload(payload, archive)
	if err != nil {
		return err
	}
	target := strings.TrimSuffix(registry, "/") + "/" + strings.TrimPrefix(h.TargetImageRef, "/")
	log.Printf("verified transfer %s (%s); pushing to %s", h.TransferID, h.ArchiveSHA256, target)
	digest, err := imagetransfer.PushOCI(archive, target, user, pass, insecure)
	if err != nil {
		return err
	}
	log.Printf("transfer complete: id=%s target=%s digest=%s", h.TransferID, target, digest)
	return nil
}
