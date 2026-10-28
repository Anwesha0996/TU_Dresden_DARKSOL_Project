package imagetransfer

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
)

const (
	maxHeaderSize  = 1 << 20
	maxArchiveSize = 64 << 30
)

type Header struct {
	TransferID       string `json:"transfer_id"`
	SourceImageRef   string `json:"source_image_ref"`
	TargetImageRef   string `json:"target_image_ref"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
	SourceDigest     string `json:"source_digest"`
	ArchiveSHA256    string `json:"archive_sha256"`
	ArchiveSizeBytes int64  `json:"archive_size_bytes"`
}

// ValidateTarget rejects destination names that could escape the configured
// registry prefix or replace it with a different registry/URL.
func ValidateTarget(target string) error {
	if target == "" {
		return fmt.Errorf("target image is empty")
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "/") {
		return fmt.Errorf("unsafe target image %q", target)
	}
	for _, part := range strings.Split(strings.ReplaceAll(target, "\\", "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe target image %q", target)
		}
	}
	return nil
}

// DefaultTarget preserves the source repository path and tag while removing
// the source registry hostname. For example, registry.k8s.io/pause:3.10 becomes
// pause:3.10 and docker.io/library/nginx:alpine becomes library/nginx:alpine.
func DefaultTarget(source string) (string, error) {
	ref, err := name.ParseReference(source)
	if err != nil {
		return "", fmt.Errorf("parse source image: %w", err)
	}
	repository := ref.Context().RepositoryStr()
	if _, ok := ref.(name.Digest); ok {
		return "", fmt.Errorf("digest source %q requires an explicit -registry-target", source)
	}
	return repository + ":" + ref.Identifier(), nil
}

// ExportOCI pulls one platform directly through the registry API (no Docker
// daemon) and writes a standard OCI image-layout tar archive.
func ExportOCI(source, osName, arch, archivePath string) (Header, error) {
	ref, err := name.ParseReference(source)
	if err != nil {
		return Header{}, fmt.Errorf("parse source image: %w", err)
	}
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithPlatform(v1.Platform{OS: osName, Architecture: arch}))
	if err != nil {
		return Header{}, fmt.Errorf("pull %s: %w", source, err)
	}
	sourceDigest, err := img.Digest()
	if err != nil {
		return Header{}, fmt.Errorf("calculate source digest: %w", err)
	}

	root, err := os.MkdirTemp(filepath.Dir(archivePath), "oci-layout-")
	if err != nil {
		return Header{}, err
	}
	defer os.RemoveAll(root)
	p, err := layout.Write(root, empty.Index)
	if err != nil {
		return Header{}, fmt.Errorf("create OCI layout: %w", err)
	}
	if err := p.AppendImage(img, layout.WithAnnotations(map[string]string{"org.opencontainers.image.ref.name": ref.Identifier()})); err != nil {
		return Header{}, fmt.Errorf("write OCI image: %w", err)
	}
	if err := tarDirectory(root, archivePath); err != nil {
		return Header{}, err
	}

	sum, size, err := hashFile(archivePath)
	if err != nil {
		return Header{}, err
	}
	return Header{TransferID: uuid.NewString(), SourceImageRef: source, CreatedAtUnix: time.Now().Unix(), SourceDigest: sourceDigest.String(), ArchiveSHA256: sum, ArchiveSizeBytes: size}, nil
}

// CreatePayload writes [8-byte big-endian header length][JSON][OCI tar].
func CreatePayload(header Header, archivePath, payloadPath string) (int64, error) {
	b, err := json.Marshal(header)
	if err != nil {
		return 0, err
	}
	if len(b) > maxHeaderSize {
		return 0, fmt.Errorf("transfer header too large")
	}
	in, err := os.Open(archivePath)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.Create(payloadPath)
	if err != nil {
		return 0, err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(payloadPath)
		}
	}()
	if err := binary.Write(out, binary.BigEndian, uint64(len(b))); err != nil {
		return 0, err
	}
	if _, err := out.Write(b); err != nil {
		return 0, err
	}
	if _, err := io.Copy(out, in); err != nil {
		return 0, err
	}
	if err := out.Close(); err != nil {
		return 0, err
	}
	ok = true
	return int64(8+len(b)) + header.ArchiveSizeBytes, nil
}

// DecodePayload extracts and verifies an OCI archive from a received payload.
func DecodePayload(payloadPath, archivePath string) (Header, error) {
	in, err := os.Open(payloadPath)
	if err != nil {
		return Header{}, err
	}
	defer in.Close()
	var n uint64
	if err := binary.Read(in, binary.BigEndian, &n); err != nil {
		return Header{}, fmt.Errorf("read header length: %w", err)
	}
	if n == 0 || n > maxHeaderSize {
		return Header{}, fmt.Errorf("invalid header length %d", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(in, b); err != nil {
		return Header{}, fmt.Errorf("read header: %w", err)
	}
	var h Header
	if err := json.Unmarshal(b, &h); err != nil {
		return Header{}, fmt.Errorf("decode header: %w", err)
	}
	if h.TransferID == "" || h.SourceImageRef == "" || h.TargetImageRef == "" || h.SourceDigest == "" || h.ArchiveSHA256 == "" {
		return Header{}, fmt.Errorf("transfer header has missing required fields")
	}
	if h.ArchiveSizeBytes <= 0 || h.ArchiveSizeBytes > maxArchiveSize {
		return Header{}, fmt.Errorf("invalid archive size %d", h.ArchiveSizeBytes)
	}
	if err := ValidateTarget(h.TargetImageRef); err != nil {
		return Header{}, err
	}
	out, err := os.Create(archivePath)
	if err != nil {
		return Header{}, err
	}
	_, copyErr := io.Copy(out, io.LimitReader(in, h.ArchiveSizeBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		return Header{}, copyErr
	}
	if closeErr != nil {
		return Header{}, closeErr
	}
	sum, size, err := hashFile(archivePath)
	if err != nil {
		return Header{}, err
	}
	if size != h.ArchiveSizeBytes {
		return Header{}, fmt.Errorf("archive size mismatch: got %d, want %d", size, h.ArchiveSizeBytes)
	}
	if !strings.EqualFold(sum, h.ArchiveSHA256) {
		return Header{}, fmt.Errorf("archive checksum mismatch: got %s, want %s", sum, h.ArchiveSHA256)
	}
	return h, nil
}

func PushOCI(archivePath, target, username, password string, insecure bool) (string, error) {
	root, err := os.MkdirTemp(filepath.Dir(archivePath), "received-oci-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	if err := untar(archivePath, root); err != nil {
		return "", err
	}
	p, err := layout.FromPath(root)
	if err != nil {
		return "", fmt.Errorf("open OCI layout: %w", err)
	}
	idx, err := p.ImageIndex()
	if err != nil {
		return "", fmt.Errorf("read OCI index: %w", err)
	}
	m, err := idx.IndexManifest()
	if err != nil || len(m.Manifests) != 1 {
		return "", fmt.Errorf("OCI archive must contain exactly one image")
	}
	img, err := p.Image(m.Manifests[0].Digest)
	if err != nil {
		return "", fmt.Errorf("read OCI image: %w", err)
	}
	var opts []name.Option
	if insecure {
		opts = append(opts, name.Insecure)
	}
	ref, err := name.ParseReference(target, opts...)
	if err != nil {
		return "", fmt.Errorf("parse Harbor target: %w", err)
	}
	remoteOpts := []remote.Option{remote.WithAuth(authn.Anonymous)}
	if username != "" || password != "" {
		remoteOpts = []remote.Option{remote.WithAuth(&authn.Basic{Username: username, Password: password})}
	}
	if err := remote.Write(ref, img, remoteOpts...); err != nil {
		return "", fmt.Errorf("push to Harbor: %w", err)
	}
	d, err := img.Digest()
	if err != nil {
		return "", err
	}
	remoteDescriptor, err := remote.Head(ref, remoteOpts...)
	if err != nil {
		return "", fmt.Errorf("verify image in destination registry: %w", err)
	}
	if remoteDescriptor.Digest != d {
		return "", fmt.Errorf("destination digest mismatch: got %s, want %s", remoteDescriptor.Digest, d)
	}
	return remoteDescriptor.Digest.String(), nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func tarDirectory(root, destination string) error {
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(out)
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

func untar(source, root string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(h.Name))
		cleanRoot := filepath.Clean(root) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target), cleanRoot) {
			return fmt.Errorf("unsafe OCI archive path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported OCI archive entry %q", h.Name)
		}
	}
}
