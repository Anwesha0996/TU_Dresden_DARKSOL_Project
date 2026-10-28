package cmd

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// configureCmd represents the configure command
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Generates a configuration file for helm-dtn commands",
	Long:  `The configure command generates a configuration file used by other helm-dtn commands.`,
	Run: func(cmd *cobra.Command, args []string) {
		var config util.Config
		var connect string

		fmt.Println("Enter the EID of the cluster charts should be deployed to:")
		fmt.Scanln(&config.DestinationEid)
		fmt.Println("Enter the address of the local registry running in the remote cluster (format: registry-address:port):")
		fmt.Scanln(&config.Registry)
		fmt.Println("Do you want to connect to µD3TN using a [s]ocket or [t]cp?")
		fmt.Scanln(&connect)
		for {
			if connect != "s" && connect != "t" {
				fmt.Println("Please enter either 's' or 't'!")
				fmt.Scanln(&connect)
				continue
			}
			break
		}
		if connect == "s" {
			fmt.Println("Enter the path to the UNIX domain socket of the AAP 2.0 service:")
			fmt.Scanln(&config.Socket)
			config.Tcp = ""
		} else {
			fmt.Println("Enter the address the AAP 2.0 service is reachable at (e.g. localhost:4242 or 127.0.0.1:4242):")
			fmt.Scanln(&config.Tcp)
			config.Socket = ""
		}
		fileWriter := &util.OSFileHandler{}
		configDir := ".local/share/helm-dtn/"

		homeDir, err := fileWriter.GetHomeDir()
		if err != nil {
			log.Fatalf("failed to create config directory: %v\n", err)
		}

		fullDirPath := filepath.Join(homeDir, configDir)

		if err := fileWriter.CreateDir(fullDirPath, 0755); err != nil {
			log.Fatalf("failed to create config directory: %v\n", err)

		}

		fullFilePath := filepath.Join(fullDirPath, "config.yaml")

		if data, err := yaml.Marshal(config); err == nil {
			if writeErr := fileWriter.Write(fullFilePath, data, 0644); writeErr != nil {
				log.Fatalf("failed to write configuration file: %v", writeErr)
			}
		}

	},
}

func init() {
	rootCmd.AddCommand(configureCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// configureCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// configureCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
