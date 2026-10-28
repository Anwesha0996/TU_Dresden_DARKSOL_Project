package helmdl

//TODO: Currently everything is saved into the directory the script is in. In the future we need to
// support alternative storage backends (other paths, S3, ...)

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/client" // Apache License, Version 2.0
	"gopkg.in/yaml.v3"
)

// DownloadDependencies fetches dependencies previously identified by helmdd.
func DownloadDependencies(dependencyList, runTempDir string) error {
	d, err := os.ReadFile(dependencyList)

	if err != nil {
		log.Printf("helmdl: failed to read dependency list: %v", err)
		return err
	}

	var deps util.Dependencies

	if err := yaml.Unmarshal(d, &deps); err != nil {
		log.Printf("helmdl: failed to unmarshal dependency list: %v", err)
		return err
	}
	imagesDir := filepath.Join(runTempDir, "Images")
	gitDir := filepath.Join(runTempDir, "Git")

	os.Mkdir(imagesDir, os.ModePerm)
	os.Mkdir(gitDir, os.ModePerm)

	if err := getImages(deps.Images, imagesDir); err != nil {
		// If there was an error while downloading the images, abort
		// -> can't deploy the chart without everything available locally
		log.Printf("helmdl: unable to save all images locally: %v", err)
		return err
	}
	if err := getGitRepos(deps.Git, gitDir); err != nil {
		log.Printf("helmdl: unable to save all git repos locally: %v", err)
		return err
	}

	return nil
}

// getImages uses Moby's docker client to pull and save images and Google's go-containerregistry
// to check whether images already exist locally
func getImages(imageMap map[string][]string, saveDir string) error {
	dockerClient, err := client.New()
	if err != nil {
		log.Printf("helmdl: failed to create Docker client: %v", err)
		return err
	}

	// Get a list of locally available images for comparison to requested images
	localImages, err := dockerClient.ImageList(context.Background(), client.ImageListOptions{})
	if err != nil {
		log.Printf("helmdl: failed to list local images: %v", err)
	}
	defer dockerClient.Close()

	// Loop through all registries and images, and check whether they
	// are already available locally
	for registry, images := range imageMap {
		for _, image := range images {
			var fullImage string
			if registry != "" {
				fullImage = registry + "/" + image
			} else {
				fullImage = image
			}
			log.Printf("helmdl: checking whether %v is available locally...", fullImage)
			digest, err := getDigest(fullImage)
			if err != nil {
				log.Printf("helmdl: unable to check digest of image %v: %v", fullImage, err)
			}

			availableLocally := false

			// Compare repoDigest of local images to digest of required image
			// This also allows for the use of the :latest tag, which would cause
			// problems if we only compared image names + version tags
			for _, v := range localImages.Items {
				// Compare local digest to remote digest to ensure they're the same
				// version.
				// -> this fails if the image was never pushed or pulled!
				// -> in this case we check if the tag of a local image matches
				if len(v.RepoDigests) > 0 {
					repoDigest := v.RepoDigests[0]
					re := regexp.MustCompile(`sha[a-zA-z0-9:]*`)
					match := re.FindStringSubmatch(repoDigest)

					if match[0] == digest {
						log.Printf("helmdl: Found image %v locally...", fullImage)
						availableLocally = true
						break
					}
				} else if slices.Contains(v.RepoTags, fullImage) {
					availableLocally = true
					break
				}
			}

			// Image is not available in the local registry
			// -> pull it
			if !availableLocally {
				if err := pullImage(fullImage, dockerClient); err != nil {
					log.Printf("helmdl: unable to pull image %v: %v", fullImage, err)
					return err
				}
			}

			if err := saveImage(fullImage, dockerClient, saveDir); err != nil {
				log.Printf("helmdl: unable to save image %v: %v", fullImage, err)
				return err
			}
		}
	}

	return nil

}

// getDigest queries a remote registry for the digest of a specified image.
func getDigest(imageTag string) (string, error) {
	ref, err := name.ParseReference(imageTag)
	if err != nil {
		return "", err
	}

	img, err := remote.Get(ref)
	if err != nil {
		return "", err
	}
	return img.Digest.String(), nil
}

// pullImage pulls the specified image using docker pull
func pullImage(fullImage string, dockerClient *client.Client) error {
	log.Printf("helmdl: pulling image %v...\n", fullImage)
	response, pullErr := dockerClient.ImagePull(context.Background(), fullImage, client.ImagePullOptions{})
	if pullErr != nil {
		log.Printf("helmdl: Could not pull image %v: %v", fullImage, pullErr)
		return pullErr
	}
	response.Wait(context.Background())

	return nil
}

// saveImage saves the specified image to disk using docker save
func saveImage(image string, dockerClient *client.Client, saveDir string) error {
	// save image without the repository being part of the filename to
	// avoid conflicts caused by "/"
	imageComponents := strings.Split(image, "/")
	// HACK: Currently assumes we want to save to the supplied directory. Should be changed to support
	// different storage backends.
	filePath := filepath.Join(saveDir, imageComponents[len(imageComponents)-1]+".tar")

	// Call ImageSave to create a ReadCloser for writing the exported image to disc.
	readCloser, err := dockerClient.ImageSave(context.Background(), strings.Split(image, " "))
	if err != nil {
		log.Printf("helmdl: failed to call ImageSave: %v", err)
		return err
	}
	defer readCloser.Close()

	outFile, err := os.Create(filePath)
	if err != nil {
		log.Printf("helmdl: failed to create output file: %v", err)
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, readCloser)
	if err != nil {
		log.Printf("helmdl: failed to write image stream to file: %v", err)
		return err
	}

	return nil
}

// getGitRepos executes git clone to fetch any repositories required by the application
func getGitRepos(gitList map[string]string, saveDir string) error {
	// XXX: The git clone commands taken from the manifests may contain paths to clone into.
	// XXX: These paths are saved in the helmdd.yaml as well, so the Application Lifecycle Management
	// XXX: tooling in the airgapped cluster knows where to put the cloned repos
	for repo := range gitList {
		// Check if we've already cloned this repo
		// XXX: Will always be false as long as the repos are saved in the run-specific temp dir as opposed to some central git storage. Needs discussion.
		gitDir := path.Base(strings.TrimSuffix(repo, ".git"))
		if directoryExists(filepath.Join(saveDir, gitDir)) {
			// Repo has been cloned before, next check if it's still the latest version
			// -> HEAD hash matches with remote
			cmd := exec.Command("git", "ls-remote", repo, "HEAD")
			out, err := cmd.Output()
			if err != nil {
				log.Fatal(err)
			}

			remoteCommitHash := strings.Fields(string(out))[0]
			localCmd := exec.Command("git", "-C", gitDir, "rev-parse", "HEAD")
			out2, err := localCmd.Output()
			if err != nil {
				log.Fatal(err)
			}

			hash := strings.TrimSpace(string(out2))

			if remoteCommitHash == hash {
				log.Printf("helmdl: Repository %v already available in latest version! Skipping pull...", repo)
				continue
			}

			// Hash didn't match
			// -> we want to clone the newer version
			// -> delete old directory
			if removeErr := os.RemoveAll(gitDir); removeErr != nil {
				log.Printf("helmdl: unable to remove directory '%v': %v", gitDir, removeErr)
			}
		}
		log.Printf("helmdl: Cloning repository %v...\n", repo)

		cmd := exec.Command("git", "clone", "--recurse-submodules", "-j8", repo, filepath.Join(saveDir, gitDir))

		stderr, _ := cmd.StderrPipe()
		if startErr := cmd.Start(); startErr != nil {
			log.Printf("helmdl: failed to clone %v: %v", repo, startErr)
			return startErr
		}

		// To show real-time output for git clone, a scanner is initialized.
		// Git seems to send its output to stderr, so we scan stderr instead of stdout.
		scanner := bufio.NewScanner(stderr)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			m := scanner.Text()
			log.Printf("helmdl:   %v", m)
		}

		if runErr := cmd.Wait(); runErr != nil {
			log.Printf("helmdl: failed to clone %v: %v", repo, runErr)
			return runErr
		}
	}
	return nil
}

// directoryExists checks whether a directory exists
func directoryExists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return true
}
