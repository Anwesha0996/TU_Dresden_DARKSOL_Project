package helmdtnt

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"gopkg.in/yaml.v3"
)

// SendImagesViaRust reads the dependency list produced by helmdd and sends
// each detected image individually over AAP2 via the Rust
// `aap2-image-sender` binary, instead of bundling image bytes into the
// chart archive.
//
// Requires `aap2-image-sender` to be on PATH. destinationEid must already be
// the full agent EID (e.g. "dtn://cluster.dtn/image-transfer") - callers
// append the image agent suffix themselves, mirroring
// how cmd/install.go and cmd/upgrade.go already build the "install"/
// "upgrade" agent EIDs.
func SendImagesViaRust(dependencyList string, cfg util.Config, destinationEid, releaseName string) error {
	d, err := os.ReadFile(dependencyList)
	if err != nil {
		return fmt.Errorf("helmdtnt: failed to read dependency list: %w", err)
	}
	var deps util.Dependencies
	if err := yaml.Unmarshal(d, &deps); err != nil {
		return fmt.Errorf("helmdtnt: failed to unmarshal dependency list: %w", err)
	}

	for registry, images := range deps.Images {
		for _, image := range images {
			sourceRef := image
			if registry != "" {
				sourceRef = registry + "/" + image
			}
			// image is already the post-last-slash component (see
			// helmdd's extractDependencies), i.e. exactly the "base
			// name:tag" helmdp's overrideRefs rewrites chart references
			// to - reusing it as-is here keeps both transports pointing
			// at the identical final registry path.
			targetRef := image

			log.Printf("helmdtnt: sending image %v via Rust AAP2 path (target: %v)...", sourceRef, targetRef)
			args := []string{
				"--destination-eid", destinationEid,
				"--image", sourceRef,
				"--harbor-target", targetRef,
				"--release", releaseName,
			}
			if cfg.Socket != "" {
				args = append(args, "--socket", cfg.Socket)
			} else {
				args = append(args, "--tcp", cfg.Tcp)
			}

			cmd := exec.Command("aap2-image-sender", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("helmdtnt: aap2-image-sender failed for %v: %w", sourceRef, err)
			}
		}
	}
	return nil
}
