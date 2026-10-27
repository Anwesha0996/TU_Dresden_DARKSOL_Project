package helmdp

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"gopkg.in/yaml.v3"
)

// BundleDependencies updates references and prepares the archive to be
// sent to a remote cluster.
func BundleDependencies(chartRoot, dependencyList, registry string) (string, error) {
	log.Println("helmdp: Bundling dependencies...")

	d, err := os.ReadFile(dependencyList)

	if err != nil {
		log.Printf("helmdp: failed to read dependency list: %v", err)
		return "", err
	}
	var deps util.Dependencies

	if err := yaml.Unmarshal(d, &deps); err != nil {
		log.Printf("helmdp: failed to unmarshal dependency list: %v", err)
		return "", err
	}
	if err := prepareRefs(chartRoot); err != nil {
		return "", fmt.Errorf("helmdp: %w", err)
	}
	if err := overrideRefs(chartRoot, registry, deps); err != nil {
		return "", err
	}

	var archivePath string
	log.Println("helmdp: Creating archive... (This may take a while)")
	if archivePath, err = util.Tar(chartRoot); err != nil {
		return "", fmt.Errorf("helmdp: %w", err)
	}

	return archivePath, nil
}

// prepareRefs walks through the directory tree and unpacks all .tgz archives so any references
// to images or other resources inside these archives can be updated as well.
func prepareRefs(root string) error {

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Propagate any error encountered during file access
			return err
		}

		if strings.Contains(d.Name(), ".tgz") || strings.Contains(d.Name(), ".tar.gz") {
			r, err := os.Open(path)
			if err != nil {
				return err
			}
			log.Printf("helmdp: Found subchart archive: %v, unpacking...", d.Name())
			if err := util.Untar(r, strings.TrimSuffix(path, filepath.Ext(path))); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

// overrideRefs walks through all files in the chart directory, updating references
// to point to local resources as needed.
func overrideRefs(chartRoot, registry string, deps util.Dependencies) error {
	log.Println("helmdp: Overriding references in chart to point to local registry...")

	err := filepath.WalkDir(chartRoot, func(pathYAML string, d fs.DirEntry, err error) error {
		if err != nil {
			// Propagate any error encountered during file access
			return err
		}

		if d.IsDir() {
			// Skip directories
			return nil
		}

		if !shouldProcessFile(pathYAML) {
			// Skip non-YAML files
			return nil
		}

		if strings.Contains(pathYAML, "template") {
			log.Printf("helmdp: can't parse %v as YAML (is template file). Using alternative approach to update...\n", pathYAML)
			if err := updateTemplate(pathYAML, registry, deps); err != nil {
				return fmt.Errorf("failed to update template %v: %w", pathYAML, err)
			}
			return nil
		}

		parsedYAML, err := parseYAML(pathYAML)
		if err != nil {
			log.Printf("helmdp: failed to parse YAML (%v): %v\n", pathYAML, err)
			// We do NOT return an error here, as there are some YAML files
			// (esp. in templates/) that would cause this to return an error,
			// which in turn would wrongly stop execution.
			return nil
		}

		walkYAML(parsedYAML.Content[0], func(n *yaml.Node) {
			nodeKind := n.Kind

			if nodeKind != yaml.MappingNode {
				if nodeKind == yaml.ScalarNode {
					if strings.Contains(n.Value, "git clone") {
						n.Value = strings.ReplaceAll(n.Value, "git clone", "cp -r")
					}

				}
				return
			}
			_, imgNode := findKeyValue(n, "image")

			if imgNode != nil {

				if imgNode.Kind == yaml.ScalarNode {
					// -> imgNode is of type "image: registry/repo:tag"

					// Don't update if imgNode contains a templated value
					// (such as image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"),
					// as we will update the values file containing the definition anyways.
					if !strings.Contains(imgNode.Value, "{") {
						// imgNode contains an actual image reference
						// -> take the base (image name + tag/digest) and prepend the local registry
						imgNode.Value = fmt.Sprintf("%v/%v", registry, path.Base(imgNode.Value))
					}
				}

				// Check if imgNode is a mapping
				if imgNode.Kind != yaml.MappingNode {
					return
				}

				_, regNode := findKeyValue(imgNode, "registry")
				_, repoNode := findKeyValue(imgNode, "repository")

				// Rewrite registry if it exists
				if regNode != nil {
					regNode.Value = registry
				}

				// Rewrite repository if it exists
				if repoNode != nil {
					// If there is a "registry" entry, we need to clean up the
					// repository value to only contain the image name.
					if regNode != nil {
						repoNode.Value = path.Base(repoNode.Value)
					} else {
						// There was no "registry" field, so the registry is most likely
						// part of the repository value.
						// -> We take the base (image name) and prepend our own registry.
						repoNode.Value = fmt.Sprintf("%v/%v", registry, path.Base(repoNode.Value))
					}
				}
			}

			// Redirect git links by removing everything but the repository names,
			// which coincides with the directories created by helmdl.
			for git := range deps.Git {
				_, gitRef := findValueKey(n, git)
				if gitRef != nil {
					gitRef.Value = strings.TrimSuffix(path.Base(git), ".git")
				}
			}

		})
		if err := writeYAML(pathYAML, parsedYAML); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("helmdp: an error occurred during chart traversal: %w", err)
	}

	log.Println("helmdp: Successfully updated references")
	return nil
}

// updateTemplate reads a Helm template (which due to the placeholders cannot be marshalled by yaml.Marshal) and updates
// references to images/git repos, ... to point to local alternatives.
func updateTemplate(path, registry string, dependencies util.Dependencies) error {
	templateBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %v: %w", path, err)
	}

	template := string(templateBytes)

	// Update image references to point to local registry.
	for remoteReg, images := range dependencies.Images {
		for _, image := range images {
			if remoteReg != "" {
				template = strings.ReplaceAll(template, remoteReg+"/"+image, registry+"/"+image)
			} else {
				template = strings.ReplaceAll(template, image, registry+"/"+image)
			}
		}
	}

	// Update git links to point to downloaded repos.
	for git := range dependencies.Git {
		template = strings.ReplaceAll(template, git, strings.TrimSuffix(filepath.Base(git), ".git"))
	}

	// Replace "git clone" commands with "cp -r" to copy downloaded repos instead of fetching remote.
	// XXX: This may result in errors if the git clone does not specify a target directory.
	// XXX: E.g. "git clone github.com/somerepo.git" would become "cp -r somerepo" which is not valid.
	template = strings.ReplaceAll(template, "git clone", "cp -r")

	// Overwrite the file with the new changes.
	if err := os.WriteFile(path, []byte(template), 0644); err != nil {
		return err
	}
	return nil
}

// shouldProcessFile compares the file extension of a file to a predefined list of extensions
// and returns a Boolean indicating whether they match.
func shouldProcessFile(path string) bool {
	extensions := []string{".yaml", ".yml"}

	for _, ext := range extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

// parseYAML parses a YAML file and returns the root node.
func parseYAML(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return &root, nil
}

// walkYAML recursively walks the YAML definition rooted at YAML node n and executes
// the passed function for each step.
func walkYAML(n *yaml.Node, fn func(*yaml.Node)) {
	fn(n)
	for _, c := range n.Content {
		walkYAML(c, fn)
	}
}

// findKeyValue finds a key in the YAML structure and returns the YAML node and the associated value.
func findKeyValue(n *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		if k.Value == key {
			return k, v
		}
	}
	return nil, nil
}

// findValueKey finds a value in the YAML structure and returns the YAML node plus the associated key.
func findValueKey(n *yaml.Node, value string) (*yaml.Node, *yaml.Node) {
	if n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		if v.Value == value {
			return k, v
		}
	}
	return nil, nil
}

// writeYAML writes a YAML struct rooted in root to the file specified by path.
func writeYAML(path string, root *yaml.Node) error {
	fileWriter := &util.OSFileHandler{}
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	if err = fileWriter.Write(path, out, 0644); err != nil {
		return err
	}
	return nil
}
