package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Untar extracts the contents of a provided .tar.gz archive.
//
// If the extracted directory contains a single top-level directory which in turn contains
// the chart definitions, it is stripped and all contents are placed in the root directory
// (similar to the --strip-components=1 option of the tar command).
func Untar(gzipStream io.Reader, destPath string) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Construct the full path for the file/directory in the destination.
		targetPath := filepath.Join(destPath, header.Name)

		// Sanitize the path to prevent "zip slip" attacks (e.g., paths like ../../etc/passwd)
		// see https://security.snyk.io/research/zip-slip-vulnerability
		if !strings.HasPrefix(targetPath, filepath.Clean(destPath)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in tar archive: %s", header.Name)
		}

		// Check the file type indicated by the header.
		switch header.Typeflag {
		case tar.TypeDir:
			// -> directory
			// Create it.
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

		case tar.TypeReg:
			// -> regular file
			// Ensure the parent directory exists before creating the file.
			parentDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return fmt.Errorf("failed to ensure parent directory %s exists: %w", parentDir, err)
			}

			// Create the output file.
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}
			defer outFile.Close()

			// Copy the file content from the tar reader to the output file.
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to copy file content to %s: %w", targetPath, err)
			}

			outFile.Close()

		// Handle other types (this should in theory never be called).
		// See https://pkg.go.dev/archive/tar#TypeDir for a full list of possible types.
		default:
			fmt.Printf("Skipping unsupported file type %v in %s\n", header.Typeflag, header.Name)
		}
	}
	log.Println("Untar(): Checking if there is a top-level directory that needs to be stripped...")
	return removeTopLevelDir(destPath)
}

// removeTopLevelDir checks the given subchart directory for the presence of a single
// top-level directory containing the chart.
//
// If one is found, it is removed, and all its contents are "promoted" to the
// root of the supplied directory.
func removeTopLevelDir(path string) error {
	const targetFile = "Chart.yaml"

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// Check for the existence of the Chart.yaml directly.
	targetFilePath := filepath.Join(path, targetFile)
	if _, err := os.Stat(targetFilePath); err == nil {
		log.Printf("   -> SUCCESS: Target file '%s' found directly. No strip required.\n", targetFile)
		// Chart.yaml found in root directory.
		// -> no further action needed
		return nil
	}

	// Check for a single top-level directory.
	if len(entries) == 1 && entries[0].IsDir() {
		topDirName := entries[0].Name()
		topDirPath := filepath.Join(path, topDirName)

		log.Printf("   -> DETECTED: Single top-level directory '%s'. Checking inside...\n", topDirName)

		// Check for Chart.yaml inside the top-level directory.
		targetFilePathInDir := filepath.Join(topDirPath, targetFile)
		if _, err := os.Stat(targetFilePathInDir); err == nil {
			// Chart.yaml found.
			// -> strip top-level directory
			log.Printf("   -> STRIPPING: Chart.yaml found inside. Moving contents to root...\n")

			stripEntries, err := os.ReadDir(topDirPath)
			if err != nil {
				return err
			}

			for _, entry := range stripEntries {
				oldPath := filepath.Join(topDirPath, entry.Name())
				newPath := filepath.Join(path, entry.Name())

				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("failed to move %s: %w", oldPath, err)
				}
			}

			// Remove the now-empty top-level directory
			if err := os.Remove(topDirPath); err != nil {
				//XXX: Should be safe to just log this without returning an error, as extraction is complete anyways
				log.Printf("   -> WARNING: Could not remove empty directory %s: %v\n", topDirPath, err)
			}

			log.Println("   -> SUCCESS: Stripping complete.")
			return nil
		}
	}

	// Chart.yaml wasn't found in either expected location, or there are multiple top-level directories.
	// -> something is wrong with the subchart
	// -> return error
	return fmt.Errorf("target file '%s' not found in the root of the archive or inside a single top-level directory", targetFile)
}

// Tar compresses the files in the specified directory in a gzipped tar archive.
func Tar(path string) (string, error) {
	out, err := os.OpenFile(filepath.Base(path)+".tgz", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}

	gzipWriter, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk the directory containing the chart and tar + gzip all found files.
	err = filepath.WalkDir(path, func(currPath string, d fs.DirEntry, err error) error {

		if err != nil {
			return err
		}

		// Get relative path inside the tar.
		rel, err := filepath.Rel(path, currPath)
		if err != nil {
			return err
		}

		if rel == "." {
			// Skip the root.
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Create header.
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Ensure forward slashes for tar.
		// Should always be the case anyways, but better safe than sorry.
		hdr.Name = filepath.ToSlash(rel)

		if err := tarWriter.WriteHeader(hdr); err != nil {
			return err
		}

		// Only copy regular files.
		if info.Mode().IsRegular() {
			f, err := os.Open(currPath)
			if err != nil {
				return err
			}

			_, err = io.Copy(tarWriter, f)
			if err != nil {
				return err
			}
			f.Close()
		}

		return nil
	})

	if err != nil {
		return "", err
	}
	return filepath.Base(path) + ".tgz", nil
}
