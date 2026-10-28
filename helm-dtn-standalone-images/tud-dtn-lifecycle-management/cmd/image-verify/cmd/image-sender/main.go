package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/aap2_client"
	"github.com/darksol-cloud/helm-dtn/pkg/imageplan"
	"github.com/darksol-cloud/helm-dtn/pkg/imagetransfer"
	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	var images stringList
	var targets stringList
	flag.Var(&images, "image", "source image reference; repeat for multiple images")
	flag.Var(&targets, "registry-target", "destination repository and tag; repeat in the same order as -image")
	imagesFile := flag.String("images-file", "", "text file with SOURCE or SOURCE=TARGET on each line")
	destination := flag.String("destination-eid", "", "remote image receiver EID")
	tcp := flag.String("tcp", "", "local micro-D3TN AAP2 address")
	workDir := flag.String("work-dir", "", "directory for temporary OCI files")
	osName := flag.String("os", "linux", "target image operating system")
	arch := flag.String("arch", "amd64", "target image architecture")
	flag.Parse()
	if *destination == "" || *tcp == "" {
		flag.Usage()
		os.Exit(2)
	}
	requests, err := imageplan.Build(images, targets, *imagesFile)
	if err != nil {
		log.Fatal(err)
	}
	if err := run(requests, *destination, *tcp, *workDir, *osName, *arch); err != nil {
		log.Fatal(err)
	}
}

func run(requests []imageplan.Request, destination, tcp, workDir, osName, arch string) error {
	var cleanup func()
	if workDir == "" {
		d, err := os.MkdirTemp("", "go-image-sender-")
		if err != nil {
			return err
		}
		workDir = d
		cleanup = func() { os.RemoveAll(d) }
	} else {
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return err
		}
		cleanup = func() {}
	}
	defer cleanup()
	client := aap2_client.NewAAP2TCPClient(tcp)
	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Disconnect()
	if _, err := client.Configure(aap2_client.WithAgentId("go-image-sender")); err != nil {
		return err
	}
	for i, request := range requests {
		archive := filepath.Join(workDir, fmt.Sprintf("image-%03d.oci.tar", i+1))
		payload := filepath.Join(workDir, fmt.Sprintf("image-%03d.payload", i+1))
		log.Printf("[%d/%d] pulling %s directly from the registry", i+1, len(requests), request.Source)
		h, err := imagetransfer.ExportOCI(request.Source, osName, arch, archive)
		if err != nil {
			return fmt.Errorf("export %s: %w", request.Source, err)
		}
		h.TargetImageRef = request.Target
		payloadSize, err := imagetransfer.CreatePayload(h, archive, payload)
		if err != nil {
			return fmt.Errorf("create payload for %s: %w", request.Source, err)
		}
		log.Printf("[%d/%d] sending %s as one AAP2 ADU (%d bytes)", i+1, len(requests), h.TransferID, payloadSize)
		if err := client.SendFile(&aap2.BundleADU{DstEid: destination, PayloadLength: uint64(payloadSize)}, payload); err != nil {
			return fmt.Errorf("send %s: %w", request.Source, err)
		}
		if _, err := client.ReceiveResponse(); err != nil {
			return fmt.Errorf("receive acceptance for %s: %w", request.Source, err)
		}
		fmt.Printf("source=%s\ntarget=%s\nsource_digest=%s\ntransfer_id=%s\narchive_sha256=%s\n", request.Source, request.Target, h.SourceDigest, h.TransferID, h.ArchiveSHA256)
		if err := os.Remove(archive); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Remove(payload); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
