# AAP2 wire-format fixture generator

Throwaway Go program (uses the **existing Go client's** own protobuf types
and marshaling, `github.com/darksol-cloud/helm-dtn/pkg/proto`) that prints
the exact length-delimited wire bytes the Go AAP2 client produces for a few
representative messages. The hex strings it prints are hardcoded as
constants in `../../src/client.rs`'s `go_fixture_tests` module - the Rust
unit tests assert the Rust client's encoder/decoder agree with these
bytes, not just with themselves.

Run from the repo root (needs the `github.com/darksol-cloud/helm-dtn` Go
module context, hence run via the `golang` Docker image rather than a bare
`go run` - see `../../../../deploy/local-test/README.md` for why no local
Go toolchain is assumed):

```sh
docker run --rm -v "$(pwd)":/src -w /src golang:1.24 \
  go run ./rust/aap2-common/testdata/gen_fixtures
```

If `pkg/aap2_client`'s message construction or wire format ever changes,
regenerate and update the constants in `client.rs` to match.
