package imageplan

import (
	"fmt"
	"os"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/imagetransfer"
)

type Request struct {
	Source string
	Target string
}

// Build validates repeated CLI values and an optional SOURCE=TARGET text file.
func Build(images, targets []string, imagesFile string) ([]Request, error) {
	if imagesFile != "" {
		fileImages, fileTargets, err := readFile(imagesFile)
		if err != nil {
			return nil, err
		}
		if len(targets) > 0 && len(fileImages) > 0 {
			return nil, fmt.Errorf("-registry-target cannot be combined with -images-file; put targets in the file")
		}
		images = append(images, fileImages...)
		targets = append(targets, fileTargets...)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("at least one -image or -images-file entry is required")
	}
	if len(targets) != 0 && len(targets) != len(images) {
		return nil, fmt.Errorf("got %d images but %d registry targets", len(images), len(targets))
	}
	requests := make([]Request, 0, len(images))
	for i, source := range images {
		source = strings.TrimSpace(source)
		target := ""
		if len(targets) > 0 {
			target = strings.TrimSpace(targets[i])
		} else {
			var err error
			target, err = imagetransfer.DefaultTarget(source)
			if err != nil {
				return nil, err
			}
		}
		if source == "" || target == "" {
			return nil, fmt.Errorf("image source and target must not be empty")
		}
		if err := imagetransfer.ValidateTarget(target); err != nil {
			return nil, err
		}
		requests = append(requests, Request{Source: source, Target: target})
	}
	return requests, nil
}

func readFile(path string) ([]string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read images file: %w", err)
	}
	var images, targets []string
	allExplicit := true
	for lineNumber, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		source, target, found := strings.Cut(line, "=")
		images = append(images, strings.TrimSpace(source))
		if found {
			targets = append(targets, strings.TrimSpace(target))
		} else {
			allExplicit = false
		}
		if found && !allExplicit {
			return nil, nil, fmt.Errorf("images file line %d mixes explicit targets with derived targets", lineNumber+1)
		}
	}
	if !allExplicit {
		targets = nil
	}
	return images, targets, nil
}
