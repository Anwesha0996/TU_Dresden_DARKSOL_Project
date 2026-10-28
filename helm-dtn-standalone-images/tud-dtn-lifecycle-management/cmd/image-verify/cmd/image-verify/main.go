package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/imageplan"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func main() {
	imagesFile := flag.String("images-file", "", "SOURCE=TARGET image-list file")
	registry := flag.String("registry", "", "destination registry and optional repository prefix")
	insecure := flag.Bool("insecure-registry", false, "allow a plain HTTP registry (testing only)")
	osName := flag.String("os", "linux", "image operating system")
	arch := flag.String("arch", "amd64", "image architecture")
	flag.Parse()
	if *imagesFile == "" || *registry == "" {
		flag.Usage()
		os.Exit(2)
	}
	requests, err := imageplan.Build(nil, nil, *imagesFile)
	if err != nil {
		log.Fatal(err)
	}
	user, password := credentials()
	failed := false
	for _, request := range requests {
		sourceDigest, err := digest(request.Source, *osName, *arch, "", "", false, true)
		if err != nil {
			log.Printf("FAIL source=%s error=%v", request.Source, err)
			failed = true
			continue
		}
		target := strings.TrimSuffix(*registry, "/") + "/" + strings.TrimPrefix(request.Target, "/")
		targetDigest, err := digest(target, *osName, *arch, user, password, *insecure, false)
		if err != nil {
			log.Printf("FAIL target=%s error=%v", target, err)
			failed = true
			continue
		}
		if sourceDigest != targetDigest {
			log.Printf("FAIL source=%s target=%s source_digest=%s target_digest=%s", request.Source, target, sourceDigest, targetDigest)
			failed = true
			continue
		}
		fmt.Printf("OK source=%s target=%s digest=%s\n", request.Source, target, targetDigest)
	}
	if failed {
		os.Exit(1)
	}
}

func credentials() (string, string) {
	value := os.Getenv("REGISTRY_CREDS")
	if value == "" {
		value = os.Getenv("HARBOR_CREDS")
	}
	if value == "" {
		return "", ""
	}
	user, password, ok := strings.Cut(value, ":")
	if !ok {
		log.Fatal("REGISTRY_CREDS must be user:password")
	}
	return user, password
}

func digest(reference, osName, arch, user, password string, insecure, keychain bool) (string, error) {
	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	ref, err := name.ParseReference(reference, nameOpts...)
	if err != nil {
		return "", err
	}
	opts := []remote.Option{remote.WithPlatform(v1.Platform{OS: osName, Architecture: arch})}
	if keychain {
		opts = append(opts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	} else if user != "" || password != "" {
		opts = append(opts, remote.WithAuth(&authn.Basic{Username: user, Password: password}))
	} else {
		opts = append(opts, remote.WithAuth(authn.Anonymous))
	}
	img, err := remote.Image(ref, opts...)
	if err != nil {
		return "", err
	}
	d, err := img.Digest()
	if err != nil {
		return "", err
	}
	return d.String(), nil
}
