// Command receiver builds the standalone ALME binary documented in
// doc/example-deployment.md: it registers the install/upgrade/uninstall
// AAP2 agents and runs until killed. See pkg/helmdtnt/receiver.go for the
// actual implementation (kept in that package because it shares it with
// SendChart/SendImagesViaRust, which cmd/install.go and cmd/upgrade.go
// import as a library).
package main

import "github.com/darksol-cloud/helm-dtn/pkg/helmdtnt"

func main() {
	helmdtnt.RunReceiver()
}
