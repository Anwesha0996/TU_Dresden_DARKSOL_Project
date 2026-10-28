package cmd

import (
	"log"
	"os"
	"path/filepath"

	"github.com/darksol-cloud/helm-dtn/pkg/util"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	config  util.Config
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dtn",
	Short: "Deploys applications over a DTN.",
	Long: `helm-dtn is a Helm plugin allowing operators to deploy
applications to an airgapped Kubernetes cluster over a DTN.

To do so, helm-dtn also finds, downloads and packages all dependencies needed to
successfully deploy the application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	configPath := filepath.Join(home, ".local/share/helm-dtn/config.yaml")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", configPath, "config file")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
