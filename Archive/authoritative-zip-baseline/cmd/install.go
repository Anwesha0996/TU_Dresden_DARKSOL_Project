package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/darksol-cloud/helm-dtn/pkg/helmdd"
	"github.com/darksol-cloud/helm-dtn/pkg/helmdl"
	"github.com/darksol-cloud/helm-dtn/pkg/helmdp"
	"github.com/darksol-cloud/helm-dtn/pkg/helmdtnt"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install <Path to directory containing chart>",
	Short: "Installs the specified chart and all dependencies.",
	Long:  `install takes a specified Helm chart and downloads subcharts, required images and other required resources. Once all dependencies are available locally, the chart is bundled and sent over a DTN to an airgapped Kubernetes cluster, where it is subsequently installed.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no path to chart provided\n")
		}

		_, err := os.Stat(cfgFile)
		if errors.Is(err, os.ErrNotExist) && cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("No configuration found in %v. Please run 'helm dtn configure' or provide a configuration using the flags listed below!\n", cfgFile)
		} else if errors.Is(err, os.ErrNotExist) && cmd.Flags().NFlag() != 0 {
			fmt.Printf("%v", config)
			// Mutually exclusive: either --socket or --tcp...
			if config.Socket != "" && config.Tcp != "" {
				return fmt.Errorf("--socket and --tcp are mutually exclusive")
			}
			// ...neither has been set
			if config.Socket == "" && config.Tcp == "" {
				return fmt.Errorf("either --socket or --tcp are required to send data")
			}

			if config.DestinationEid == "" {
				return fmt.Errorf("destination-eid is required")
			}

			if config.Registry == "" {
				return fmt.Errorf("registry is required")
			}
			return nil
		} else {
			configFile, err := os.ReadFile(cfgFile)
			err = yaml.Unmarshal(configFile, &config)
			if err != nil {
				return fmt.Errorf("error parsing configuration file: %v", err)
			}
			return nil
		}

	},
	Run: func(cmd *cobra.Command, args []string) {
		localChart := args[0]
		log.Printf("Installing helm chart '%v'", localChart)

		log.Print("Creating temporary directory for storing dependencies...")
		runTempDir, err := os.MkdirTemp("", "helmdtn")
		if err != nil {
			log.Printf("Error: failed to create directory: %v! Aborting.", err)
		}

		dependencyPath, resolveErr := helmdd.ResolveDependencies(localChart, runTempDir)
		if resolveErr != nil {
			log.Fatalf("Error: failed to resolve dependencies: %v! Aborting.", resolveErr)
		}

		if err := helmdl.DownloadDependencies(dependencyPath, runTempDir); err != nil {
			log.Fatalf("Error: failed to download dependencies: %v! Aborting.", err)
		}
		archivePath, bundleErr := helmdp.BundleDependencies(runTempDir, dependencyPath, config.Registry)
		if bundleErr != nil {
			log.Fatalf("Error: failed to bundle dependencies: %v! Aborting.", bundleErr)
		}
		if config.DestinationEid[len(config.DestinationEid)-1:] == "/" {
			if err := helmdtnt.SendChart(config.Socket, archivePath, config.Tcp, config.DestinationEid+"install"); err != nil {
				log.Fatalf("Error: failed to transmit chart: %v! Aborting.", err)
			}
		} else {
			if err := helmdtnt.SendChart(config.Socket, archivePath, config.Tcp, config.DestinationEid+"/install"); err != nil {
				log.Fatalf("Error: failed to transmit chart: %v! Aborting.", err)
			}
		}

		log.Println("Removing temporary directory and archived dependencies...")
		if err := os.RemoveAll(runTempDir); err != nil {
			log.Printf("Error: failed to remove temporary directory: %v", err)
		}
		if err := os.RemoveAll(archivePath); err != nil {
			log.Printf("Error: failed to remove dependency archive: %v", err)
		}
		log.Printf("Successfully deployed %v!", localChart)
	},
}

func init() {
	rootCmd.AddCommand(installCmd)

	installCmd.Flags().StringVarP(&config.DestinationEid, "destination-eid", "d", "", "EID of the Application Lifecycle Management Engine in the remote cluster.")
	installCmd.Flags().StringVarP(&config.Registry, "registry", "r", "", "Name of the local registry running in the remote cluster.")
	installCmd.Flags().StringVarP(&config.Socket, "socket", "s", "", "Path to the unix socket created by µD3TN.")
	installCmd.Flags().StringVarP(&config.Tcp, "tcp", "t", "", "IP and Port µD3TN listens for incoming AAP2 connections on. Example: 127.0.0.1:4244.")
}
