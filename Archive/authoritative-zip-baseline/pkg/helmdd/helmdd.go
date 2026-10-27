// Package helmdd provides utility for resolving dependencies of Helm Charts.
package helmdd

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
)

// ResolveDependencies extracts dependencies that need to be pre-downloaded from a provided Chart and its subcharts.
func ResolveDependencies(localChartPath, runTempDir string) (string, error) {
	chartDir := filepath.Join(runTempDir, "Chart")
	if err := os.CopyFS(chartDir, os.DirFS(localChartPath)); err != nil {
		log.Printf("helmdd: failed to copy chart to temporary directory: %v", err)
		return "", err
	}
	manifest, renderErr := renderTemplate(chartDir)
	if renderErr != nil {
		log.Printf("helmdd: failed to render manifests: %v", renderErr)
		return "", renderErr
	}

	dependenciesMap := extractDependenciesFromManifests(manifest)

	fileWriter := &util.OSFileHandler{}
	exportPath, exportErr := exportDependencyListYAML(dependenciesMap, fileWriter)
	if exportErr != nil {
		log.Printf("helmdd: failed to export dependency list: %v", exportErr)
		return "", exportErr
	}

	log.Println("helmdd: Successfully resolved dependencies!")
	return exportPath, nil
}

// updateSubcharts checks for the existence of the required subcharts and invokes `helm dependency update` should any be missing.
func updateSubcharts(chart *chart.Chart, installClient *action.Install, localChartPath string) error {
	if chartDependencies := chart.Metadata.Dependencies; chartDependencies != nil {
		if err := action.CheckDependencies(chart, chartDependencies); err != nil {
			if !installClient.DependencyUpdate {
				log.Printf("helmdd: %v", err)
				log.Println("helmdd: Trying to get subcharts using `helm dependency update`...")
				cmd := exec.Command("helm", "dependency", "update", localChartPath)

				if _, err := cmd.Output(); err != nil {
					return err
				}
			}
		}
	}
	log.Println("helmdd: Successfully gathered subcharts!")
	return nil
}

// renderTemplate renders a provided Chart and returns a multi-manifest YAML string for further processing.
func renderTemplate(localChartPath string) (string, error) {
	// Basic setup
	settings := cli.New()
	actionConfig := new(action.Configuration)

	// Initialize the actionConfig for later use with the "Install" action.
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		log.Printf("helmdd: failed to init config: %v", err)
		return "", err
	}

	// Load the chart from the local path.
	chart, err := loader.Load(localChartPath)
	if err != nil {
		log.Printf("helmdd: failed to load chart: %v", err)
		return "", err
	}

	// Create and configure the Install client for template generation.
	installClient := action.NewInstall(actionConfig)

	// Configure the client for generating the template
	installClient.DryRun = true // We do not actually want to install, we just need the combined manifests
	installClient.ReleaseName = "depresolver-dry-run-release"
	installClient.Namespace = "default"
	installClient.ClientOnly = true // Do not attempt to connect to Kubernetes
	installClient.DisableHooks = true
	installClient.Replace = true // Allows reusing the release name

	log.Println("helmdd: Checking subcharts...")
	if updateErr := updateSubcharts(chart, installClient, localChartPath); updateErr != nil {
		log.Printf("helmdd: unable to update subcharts: %v", updateErr)
		return "", updateErr
	}

	// If updateSubcharts had to pull subcharts, the main chart is now outdated
	// Therefore, we reload the chart from the local path.
	chart, err = loader.Load(localChartPath)
	if err != nil {
		log.Printf("helmdd: failed to reload chart: %v", err)
		return "", err
	}

	log.Println("helmdd: Rendering manifests...")

	// The release object will contain the rendered manifest in release.Manifest
	// Since we do not want to overwrite values.yaml, the last arg in this call is nil.
	release, err := installClient.Run(chart, nil)
	if err != nil {
		log.Printf("helmdd: failed to render chart templates: %v", err)
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(release.Manifest)

	for _, v := range release.Hooks {
		sb.WriteString("\n---\n")
		sb.WriteString(v.Manifest)
	}

	return sb.String(), nil
}

// extractDependencies recursively searches for the "image" key in a generic data structure.
func extractDependencies(data any, images map[string]map[string]struct{}, gitUrls map[string]string) {
	if data == nil {
		return
	}

	switch v := data.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "image" {
				if image, ok := val.(string); ok && image != "" {
					lastInd := strings.LastIndex(image, "/")
					if lastInd != -1 {
						if images[image[:lastInd]] == nil {
							images[image[:lastInd]] = make(map[string]struct{})
						}
						images[image[:lastInd]][image[lastInd+1:]] = struct{}{}
					} else {
						if images[""] == nil {
							images[""] = make(map[string]struct{})
						}
						images[""][image] = struct{}{}
					}
				}
			}
			// Recurse into nested map values
			extractDependencies(val, images, gitUrls)
		}
	case map[any]any: // Handle non-string keys
		for _, val := range v {
			extractDependencies(val, images, gitUrls)
		}
	case []any:
		for _, item := range v {
			if val_string, ok := item.(string); ok && strings.Contains(val_string, "git clone") {

				// Match the git clone command and group it by its parts (command, git url, path to clone into)
				re := regexp.MustCompile(`(git clone\s)(https://.*.git)\s(.*)\s*`)
				match := re.FindStringSubmatch(val_string)
				// Check for the existence of a path the repo should be cloned into
				if len(match) == 4 {
					// re.FindStringSubmatch returns the leftmost match at index 0, followed by the matched groups.
					// Here: ["git clone https://abc.def/hij.git some/path", "git clone", "https://abc.def/hij.git", "some/path"]
					gitUrls[match[2]] = match[3]
				} else {
					// No path specified
					gitUrls[match[2]] = ""
				}
			}
			extractDependencies(item, images, gitUrls)
		}
		// For other types (string, int, bool), do nothing
	}
}

// extractDependenciesFromManifests processes the multi-document YAML string returned by renderTemplate and returns a list of unique images.
func extractDependenciesFromManifests(manifests string) map[string]any {
	// Split by YAML document separator
	manifestParts := strings.Split(manifests, "\n---")

	// Go does not have a set data structure, so we're using a map of empty structs instead.
	// This is also very memory-friendly, as empty structs occupy zero bytes of storage.
	imagesSet := make(map[string]map[string]struct{})
	uniqueGitUrls := make(map[string]string)

	for _, part := range manifestParts {
		if strings.TrimSpace(part) == "" {
			continue // Skip empty parts
		}

		// Use a generic interface to unmarshal the YAML
		var manifest map[string]any

		if err := yaml.Unmarshal([]byte(part), &manifest); err != nil {
			log.Printf("helmdd: failed to unmarshal YAML document: %v\n", err)
			continue
		}

		// Search for dependencies in the unmarshaled structure
		extractDependencies(manifest, imagesSet, uniqueGitUrls)
	}

	// Due to there not being a set data structure in go, the imagesSet map contains a map of empty structs.
	// We need to clean it up so that it becomes a list of unique images per registry.
	uniqueImages := make(map[string][]string)
	var allImages []string

	for registry, registryImages := range imagesSet {
		allImages = nil
		for image := range registryImages {
			allImages = append(allImages, image)
		}
		uniqueImages[registry] = allImages
	}

	dependencies := map[string]any{
		"images": uniqueImages,
		"git":    uniqueGitUrls,
	}

	return dependencies
}

// exportDependencyListYAML marshals the collected dependencies into YAML and writes it to a file.
func exportDependencyListYAML(dependencies map[string]any, handler util.FileHandler) (string, error) {
	const dataSubDir = ".local/share/helm-dtn"
	const fileName = "helmdd.yaml"

	data, marshalErr := yaml.Marshal(dependencies)

	if marshalErr != nil {
		log.Printf("helmdd: failed to marshall dependencies map: %v\n", marshalErr)
		return "", marshalErr
	}

	homeDir, err := handler.GetHomeDir()
	if err != nil {
		log.Printf("helmdd: failed to create data directory: %v\n", err)
		return "", err
	}

	fullDirPath := filepath.Join(homeDir, dataSubDir)

	if err := handler.CreateDir(fullDirPath, 0755); err != nil {
		log.Printf("helmdd: failed to create data directory: %v\n", err)
		return "", err
	}

	fullFilePath := filepath.Join(fullDirPath, fileName)

	if writeErr := handler.Write(fullFilePath, data, 0644); writeErr != nil {
		log.Printf("helmdd: failed to write YAML file: %v", writeErr)
		return "", writeErr
	}

	return fullFilePath, nil
}
