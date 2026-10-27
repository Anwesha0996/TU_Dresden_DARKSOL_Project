package helmdtnt

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/darksol-cloud/helm-dtn/pkg/aap2_client"
	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
)

func main() {
	var dtnServiceAdress string
	var dtnServicePort string
	var registryIP string
	var registryPort string
	var aap2Secret string

	if dtnServiceAdress = os.Getenv("DTN_SERVICE_ADRESS"); dtnServiceAdress == "" {
		panic("Missing DTN service adress!")
	}
	if dtnServicePort = os.Getenv("DTN_SERVICE_PORT"); dtnServicePort == "" {
		panic("Missing DTN service port!")
	}
	if registryIP = os.Getenv("REGISTRY_IP"); registryIP == "" {
		panic("Missing registry IP!")
	}
	if registryPort = os.Getenv("REGISTRY_PORT"); registryPort == "" {
		panic("Missing registry port!")
	}
	secretBytes, err := os.ReadFile("/etc/secret-volume/token")
	if err != nil {
		panic(err)
	}
	aap2Secret = strings.TrimSpace(string(secretBytes))
	if aap2Secret == "" {
		panic("Missing AAP2 Secret!")
	}

	wd, _ := os.Getwd()
	if err := os.Mkdir(filepath.Join(wd, "deployment"), os.ModePerm); err != nil {
		panic(err)
	}
	depDir := filepath.Join(wd, "deployment")

	go registerInstallAgent(dtnServiceAdress, dtnServicePort, aap2Secret, depDir, registryIP, registryPort)
	go registerUpgradeAgent(dtnServiceAdress, dtnServicePort, aap2Secret, depDir, registryIP, registryPort)
	// call registerUninstallAgent as regular function, so main() doesn't return
	registerUninstallAgent(dtnServiceAdress, dtnServicePort, aap2Secret)

}

func registerInstallAgent(dtnServiceAdress, dtnServicePort, aap2Secret, depDir, registryIP, registryPort string) {
	log.Println("Registering install endpoint...")
	installClient := aap2_client.NewAAP2TCPClient(dtnServiceAdress + ":" + dtnServicePort)
	if err := installClient.Connect(); err != nil {
		fmt.Println(err.Error())
		return
	}
	defer installClient.Disconnect()

	installClient.Configure(aap2_client.WithAgentId("install"), aap2_client.WithSubscribe(true), aap2_client.WithSecret(aap2Secret))

	for {
		// FIXME: For some reason the handleCommand function does not actually handle incoming commands. Needs investigation.
		//handleCommand(*installClient, depDir, registryIP, registryPort, "install")

		aduMsg, err := installClient.ReceiveMsg()
		if err != nil {
			panic(err)
		}
		if err := installClient.SendResponseStatus(aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS); err != nil {
			panic(err)
		}

		adu, recvPayload, err := installClient.ReceiveAdu(aduMsg.GetAdu())

		fmt.Printf("Received ADU from %v: %v\n", adu.SrcEid, adu.PayloadLength)
		util.Untar(bytes.NewBuffer(recvPayload), depDir)
		imgDir := filepath.Join(depDir, "Images")
		dirEntries, _ := os.ReadDir(imgDir)
		for _, entry := range dirEntries {
			log.Printf("Loading %v", filepath.Join(imgDir, entry.Name()))

			tarPath := filepath.Join(imgDir, entry.Name())
			targetRef := registryIP + ":" + registryPort + "/" + strings.TrimSuffix(entry.Name(), ".tar")

			ref, err := name.ParseReference(targetRef, name.Insecure)
			if err != nil {
				log.Fatalf("failed to parse target: %v", err)
			}

			img, err := tarball.ImageFromPath(tarPath, nil)
			if err != nil {
				log.Fatalf("failed to read tarball: %v", err)
			}

			fmt.Printf("Pushing %s to %s...\n", tarPath, targetRef)

			err = remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				log.Fatalf("failed to push image: %v", err)
			}

			fmt.Println("Successfully pushed!")
		}
		if err := installChart(filepath.Join(depDir, "Chart")); err != nil {
			panic(err)
		}
	}

}

func handleCommand(client aap2_client.AAP2TCPClient, depDir, registryIP, registryPort, commandType string) {
	log.Printf("Handling command of type %v", commandType)
	aduMsg, err := client.ReceiveMsg()
	if err != nil {
		panic(err)
	}
	if err := client.SendResponseStatus(aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS); err != nil {
		panic(err)
	}

	adu, recvPayload, err := client.ReceiveAdu(aduMsg.GetAdu())

	fmt.Printf("Received ADU from %v: %v\n", adu.SrcEid, adu.PayloadLength)
	if commandType != "uninstall" {
		util.Untar(bytes.NewBuffer(recvPayload), depDir)
		imgDir := filepath.Join(depDir, "Images")
		dirEntries, _ := os.ReadDir(imgDir)
		for _, entry := range dirEntries {
			log.Printf("Loading %v", filepath.Join(imgDir, entry.Name()))

			tarPath := filepath.Join(imgDir, entry.Name())
			targetRef := registryIP + ":" + registryPort + "/" + strings.TrimSuffix(entry.Name(), ".tar")

			ref, err := name.ParseReference(targetRef, name.Insecure)
			if err != nil {
				log.Fatalf("failed to parse target: %v", err)
			}

			img, err := tarball.ImageFromPath(tarPath, nil)
			if err != nil {
				log.Fatalf("failed to read tarball: %v", err)
			}

			fmt.Printf("Pushing %s to %s...\n", tarPath, targetRef)

			err = remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				log.Fatalf("failed to push image: %v", err)
			}

			fmt.Println("Successfully pushed!")
		}
	}

	if commandType == "install" {
		if err := installChart(filepath.Join(depDir, "Chart")); err != nil {
			panic(err)
		}
	} else if commandType == "upgrade" {
		if err := upgradeChart(filepath.Join(depDir, "Chart")); err != nil {
			panic(err)
		}
	} else if commandType == "uninstall" {
		if err := uninstallChart(string(recvPayload)); err != nil {
			panic(err)
		}
	}
}

func registerUpgradeAgent(dtnServiceAdress, dtnServicePort, aap2Secret, depDir, registryIP, registryPort string) {
	log.Println("Registering upgrade endpoint...")
	upgradeClient := aap2_client.NewAAP2TCPClient(dtnServiceAdress + ":" + dtnServicePort)
	if err := upgradeClient.Connect(); err != nil {
		fmt.Println(err.Error())
		return
	}
	defer upgradeClient.Disconnect()

	upgradeClient.Configure(aap2_client.WithAgentId("upgrade"), aap2_client.WithSubscribe(true), aap2_client.WithSecret(aap2Secret))

	for {
		// FIXME: For some reason the handleCommand function does not actually handle incoming commands. Needs investigation.
		//handleCommand(*upgradeClient, depDir, registryIP, registryPort, "upgrade")

		aduMsg, err := upgradeClient.ReceiveMsg()
		if err != nil {
			panic(err)
		}
		if err := upgradeClient.SendResponseStatus(aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS); err != nil {
			panic(err)
		}

		adu, recvPayload, err := upgradeClient.ReceiveAdu(aduMsg.GetAdu())

		fmt.Printf("Received ADU from %v: %v\n", adu.SrcEid, adu.PayloadLength)

		util.Untar(bytes.NewBuffer(recvPayload), depDir)
		imgDir := filepath.Join(depDir, "Images")
		dirEntries, _ := os.ReadDir(imgDir)
		for _, entry := range dirEntries {
			log.Printf("Loading %v", filepath.Join(imgDir, entry.Name()))

			tarPath := filepath.Join(imgDir, entry.Name())
			targetRef := registryIP + ":" + registryPort + "/" + strings.TrimSuffix(entry.Name(), ".tar")

			ref, err := name.ParseReference(targetRef, name.Insecure)
			if err != nil {
				log.Fatalf("failed to parse target: %v", err)
			}

			img, err := tarball.ImageFromPath(tarPath, nil)
			if err != nil {
				log.Fatalf("failed to read tarball: %v", err)
			}

			fmt.Printf("Pushing %s to %s...\n", tarPath, targetRef)

			err = remote.Write(ref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				log.Fatalf("failed to push image: %v", err)
			}

			fmt.Println("Successfully pushed!")
		}

		if err := upgradeChart(filepath.Join(depDir, "Chart")); err != nil {
			panic(err)
		}
	}
}

func registerUninstallAgent(dtnServiceAdress, dtnServicePort, aap2Secret string) {
	log.Println("Registering uninstall endpoint...")
	uninstallClient := aap2_client.NewAAP2TCPClient(dtnServiceAdress + ":" + dtnServicePort)
	if err := uninstallClient.Connect(); err != nil {
		fmt.Println(err.Error())
		return
	}
	defer uninstallClient.Disconnect()

	uninstallClient.Configure(aap2_client.WithAgentId("uninstall"), aap2_client.WithSubscribe(true), aap2_client.WithSecret(aap2Secret))

	for {
		// FIXME: For some reason the handleCommand function does not actually handle incoming commands. Needs investigation.
		//handleCommand(*upgradeClient, depDir, registryIP, registryPort, "uninstall")
		aduMsg, err := uninstallClient.ReceiveMsg()
		if err != nil {
			panic(err)
		}
		if err := uninstallClient.SendResponseStatus(aap2.ResponseStatus_RESPONSE_STATUS_SUCCESS); err != nil {
			panic(err)
		}

		adu, recvPayload, err := uninstallClient.ReceiveAdu(aduMsg.GetAdu())

		fmt.Printf("Received ADU from %v: %v\n", adu.SrcEid, recvPayload)
		if err := uninstallChart(string(recvPayload)); err != nil {
			panic(err)
		}
	}
}

func installChart(localChartPath string) error {
	actionConfig, chart, err := setupChartAction(localChartPath)

	if err != nil {
		return err
	}

	// Create and configure the Install client for template generation.
	installClient := action.NewInstall(actionConfig)

	// Configure the client for generating the template
	installClient.DryRun = false
	installClient.ReleaseName = chart.Name() + "-release"
	installClient.Namespace = "default"
	installClient.Replace = true // Allows reusing the release name

	log.Println("Rendering manifests...")

	// Since we do not want to overwrite values.yaml, the last arg in this call is nil.
	if _, err := installClient.Run(chart, nil); err != nil {
		log.Printf("Error: failed to render chart templates: %v", err)
		return err
	}

	return nil
}

func upgradeChart(localChartPath string) error {
	actionConfig, chart, err := setupChartAction(localChartPath)

	if err != nil {
		return err
	}

	// Create and configure the Install client for template generation.
	upgradeClient := action.NewUpgrade(actionConfig)

	// Configure the client for generating the template
	upgradeClient.DryRun = false
	upgradeClient.Namespace = "default"

	log.Println("Rendering manifests...")

	// Since we do not want to overwrite values.yaml, the last arg in this call is nil.
	if _, err := upgradeClient.Run(chart.Name()+"-release", chart, nil); err != nil {
		log.Printf("Error: failed to render chart templates: %v", err)
		return err
	}

	return nil
}

func uninstallChart(releaseName string) error {
	actionConfig, _, err := setupChartAction("")

	if err != nil {
		return err
	}
	// Create and configure the Install client for template generation.
	uninstallClient := action.NewUninstall(actionConfig)

	// Configure the client for generating the template
	uninstallClient.DryRun = false

	// Since we do not want to overwrite values.yaml, the last arg in this call is nil.
	if _, err := uninstallClient.Run(releaseName); err != nil {
		log.Printf("Error: failed to uninstall release %v: %v", releaseName, err)
		return err
	}

	return nil
}

func setupChartAction(localChartPath string) (*action.Configuration, *chart.Chart, error) {
	// Basic setup
	settings := cli.New()
	actionConfig := new(action.Configuration)

	// Initialize the actionConfig for later use with the "Install" action.
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		log.Printf("Error: failed to init config: %v", err)
		return nil, nil, err
	}

	if localChartPath != "" {
		// Load the chart from the local path.
		chart, err := loader.Load(localChartPath)
		if err != nil {
			log.Printf("Error: failed to load chart: %v", err)
			return actionConfig, nil, err
		}

		return actionConfig, chart, nil
	}

	return actionConfig, nil, nil
}
