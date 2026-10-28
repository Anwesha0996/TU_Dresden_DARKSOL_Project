# Deletion and replacement record

This repository was initialized as a local Git repository after the
Rust-native AAP2 image-channel work was completed. Git therefore cannot show
the following pre-initialization removals as ordinary deleted-file entries.
They are recorded here and in the initial commit message.

## Removed files and paths

- `deploy/local-test/scripts/test-fault-injection.sh`: removed because it
  tested the retry, quarantine and idempotency framework excluded from the
  revised scope.
- `rust/aap2-image-send/`: replaced by the correctly named
  `rust/aap2-image-sender/` crate.
- `rust/aap2-image-recv/`: replaced by the correctly named
  `rust/aap2-image-receiver/` crate.

## Removed Go behavior

- Removed the `--image-transport` install and upgrade option and its legacy
  Go image-bundle fallback.
- Removed `helmdl.DownloadDependencies`, `getImages`, `getDigest`,
  `pullImage`, and `saveImage`. Go no longer downloads or serializes container
  images.
- Removed all three `tarball.ImageFromPath` / `remote.Write` image-push blocks
  from `pkg/helmdtnt/receiver.go`.
- Removed the receiver's registry host, registry port and image-push
  credential plumbing. The Go receiver now owns chart lifecycle only.

## Removed Rust behavior

- Removed the receiver's `--registry-creds-file` and `--state-dir` options.
  Harbor credentials now enter through `HARBOR_CREDS` and are exposed to
  `skopeo` through a temporary auth file rather than process arguments.
- Removed persistent completion tracking, duplicate-delivery suppression,
  quarantine directories and metadata sidecars.
- Removed `RetryPolicy`, `run_with_retry` and the retry/backoff tests from the
  shared `skopeo` wrapper.

The resulting architecture is described in `REPORT.md` and
`08-integration-decision.md` in this directory.
