package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/darksol-cloud/helm-dtn/pkg/helmdtnt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// uninstallCmd represents the uninstall command
var uninstallCmd = &cobra.Command{
	Use:   "uninstall <release-name>",
	Short: "Takes the specified release name and uninstalls the release.",
	Long:  `uninstall takes a release name and uninstalls the release. It removes all of the resources associated with the last release of the chart as well as the release history, freeing it up for future use.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no release name specified\n")
		}

		_, err := os.Stat(cfgFile)
		if errors.Is(err, os.ErrNotExist) && cmd.Flags().NFlag() == 0 {
			return fmt.Errorf("No configuration found in %v. Please run 'helm dtn configure' or provide a configuration using the flags listed below!\n", cfgFile)
		} else if errors.Is(err, os.ErrNotExist) && cmd.Flags().NFlag() != 0 {
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
		if config.DestinationEid[len(config.DestinationEid)-1:] == "/" {
			helmdtnt.SendUninstall(args[0], config.Socket, config.Tcp, config.DestinationEid+"uninstall")
		} else {
			helmdtnt.SendUninstall(args[0], config.Socket, config.Tcp, config.DestinationEid+"/uninstall")
		}
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)

	uninstallCmd.Flags().StringVarP(&config.DestinationEid, "destination-eid", "d", "", "EID of the Application Lifecycle Management Engine in the remote cluster.")
	uninstallCmd.Flags().StringVarP(&config.Socket, "socket", "s", "", "Path to the unix socket created by µD3TN.")
	uninstallCmd.Flags().StringVarP(&config.Tcp, "tcp", "t", "", "IP and Port µD3TN listens for incoming AAP2 connections on. Example: 127.0.0.1:4244.")
}
