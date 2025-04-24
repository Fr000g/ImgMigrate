package cmd

import (
	"fmt"
	"os"

	"github.com/Fr000g/ImgMigrate/pkg/config"
	"github.com/Fr000g/ImgMigrate/pkg/docker"
	"github.com/spf13/cobra"
)

var (
	sourceImage      string
	targetImage      string
	registryURL      string
	architectures    []string
	operatingSystems []string
	outputDir        string
	allArch          bool
	username         string
	password         string
	insecure         bool
	useCompression   bool
	configFile       string
	generateConfig   string
	createMultiArch  bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "imgMigrate",
	Short: "A tool for handling multi-architecture Docker images",
	Long: `A CLI tool that can pull multi-architecture Docker images, 
tag them differently and save them locally or push to a private registry.`,
}

// pullCmd represents the pull command
var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull images from DockerHub and save locally with different tags",
	RunE: func(cmd *cobra.Command, args []string) error {
		if sourceImage == "" {
			return fmt.Errorf("source image is required")
		}

		client, err := docker.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create docker client: %v", err)
		}

		options := docker.SaveOptions{
			UseCompression:   useCompression,
			OutputDir:        outputDir,
			OperatingSystems: operatingSystems,
			CreateMultiArch:  createMultiArch,
		}

		if allArch {
			return client.PullAllArchitectures(sourceImage, options)
		}

		if len(architectures) == 0 {
			return fmt.Errorf("at least one architecture must be specified if --all-arch is not used")
		}

		return client.PullSpecificArchitectures(sourceImage, architectures, options)
	},
}

// pushCmd represents the push command
var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Pull images from DockerHub, retag and push to private registry",
	RunE: func(cmd *cobra.Command, args []string) error {
		if sourceImage == "" || targetImage == "" {
			return fmt.Errorf("source and target images are required")
		}

		client, err := docker.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create docker client: %v", err)
		}

		auth := docker.RegistryAuth{
			Username: username,
			Password: password,
			URL:      registryURL,
			Insecure: insecure,
		}

		options := docker.SaveOptions{
			OperatingSystems: operatingSystems,
			CreateMultiArch:  createMultiArch,
		}

		if allArch {
			return client.PushAllArchitectures(sourceImage, targetImage, auth, options)
		}

		if len(architectures) == 0 {
			return fmt.Errorf("at least one architecture must be specified if --all-arch is not used")
		}

		return client.PushSpecificArchitectures(sourceImage, targetImage, architectures, auth, options)
	},
}

// configCmd represents the config-based command
var configCmd = &cobra.Command{
	Use:   "from-config",
	Short: "Process images based on a YAML configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if we need to generate a sample config
		if generateConfig != "" {
			if err := config.GenerateSampleConfig(generateConfig); err != nil {
				return fmt.Errorf("failed to write sample config: %v", err)
			}
			return nil
		}

		// Load configuration from file
		if configFile == "" {
			return fmt.Errorf("config file path is required")
		}

		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %v", err)
		}

		// Process each task in the configuration
		client, err := docker.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create docker client: %v", err)
		}

		// Initialize registry auth only if registry config is provided
		var auth docker.RegistryAuth
		if cfg.Registry != nil {
			auth = docker.RegistryAuth{
				Username: cfg.Registry.Username,
				Password: cfg.Registry.Password,
				URL:      cfg.Registry.URL,
				Insecure: cfg.Registry.Insecure,
			}
		}

		for i, task := range cfg.ImageTask {
			fmt.Printf("Processing task %d: %s\n", i+1, task.Source)

			options := docker.SaveOptions{
				UseCompression:   task.Compress,
				OutputDir:        task.OutputDir,
				OperatingSystems: task.OperatingSystems,
				CreateMultiArch:  task.CreateMultiArch,
			}

			// Set default OS if not specified
			if len(options.OperatingSystems) == 0 {
				options.OperatingSystems = []string{"linux"}
			}

			// Determine whether to push or save based on target and save options
			if task.Target != "" {
				if task.AllArchitecture {
					err = client.PushAllArchitectures(task.Source, task.Target, auth, options)
				} else if len(task.Architectures) > 0 {
					err = client.PushSpecificArchitectures(task.Source, task.Target, task.Architectures, auth, options)
				} else {
					err = fmt.Errorf("task %d: either all_architectures must be true or architectures must be specified", i+1)
				}
			} else if task.Save {
				if task.AllArchitecture {
					err = client.PullAllArchitectures(task.Source, options)
				} else if len(task.Architectures) > 0 {
					err = client.PullSpecificArchitectures(task.Source, task.Architectures, options)
				} else {
					err = fmt.Errorf("task %d: either all_architectures must be true or architectures must be specified", i+1)
				}
			} else {
				err = fmt.Errorf("task %d: either target must be specified or save must be true", i+1)
			}

			if err != nil {
				fmt.Printf("Error processing task %d: %v\n", i+1, err)
				// Continue with other tasks
				continue
			}

			fmt.Printf("Successfully completed task %d\n", i+1)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(configCmd)

	// Common flags for pull command
	pullCmd.Flags().StringVarP(&sourceImage, "source", "s", "", "Source image to pull (required)")
	pullCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "Output directory for saving images")
	pullCmd.Flags().StringSliceVarP(&architectures, "arch", "a", []string{"amd64", "arm64"}, "Architectures to pull (e.g., amd64,arm64)")
	pullCmd.Flags().StringSliceVarP(&operatingSystems, "os", "", []string{"linux"}, "Operating systems to pull (e.g., linux,windows)")
	pullCmd.Flags().BoolVar(&allArch, "all-arch", false, "Pull all available architectures")
	pullCmd.Flags().BoolVarP(&useCompression, "compress", "z", false, "Use gzip compression for saved images (.tar.gz)")
	pullCmd.Flags().BoolVar(&createMultiArch, "create-multi-arch", true, "Create a multi-architecture image with -allarch tag")

	// Flags for push command
	pushCmd.Flags().StringVarP(&sourceImage, "source", "s", "", "Source image to pull (required)")
	pushCmd.Flags().StringVarP(&targetImage, "target", "t", "", "Target image name with tag (required)")
	pushCmd.Flags().StringVarP(&registryURL, "registry", "r", "", "URL of the private registry")
	pushCmd.Flags().StringSliceVarP(&architectures, "arch", "a", []string{"amd64", "arm64"}, "Architectures to pull (e.g., amd64,arm64)")
	pushCmd.Flags().StringSliceVarP(&operatingSystems, "os", "", []string{"linux"}, "Operating systems to pull (e.g., linux,windows)")
	pushCmd.Flags().BoolVar(&allArch, "all-arch", false, "Pull all available architectures")
	pushCmd.Flags().StringVarP(&username, "username", "u", "", "Username for registry authentication")
	pushCmd.Flags().StringVarP(&password, "password", "p", "", "Password for registry authentication")
	pushCmd.Flags().BoolVar(&insecure, "insecure", false, "Allow insecure registry connections")
	pushCmd.Flags().BoolVar(&createMultiArch, "create-multi-arch", true, "Create a multi-architecture image with -allarch tag")

	// Flags for config command
	configCmd.Flags().StringVarP(&configFile, "file", "f", "", "Path to the YAML configuration file")
	configCmd.Flags().StringVarP(&generateConfig, "generate", "g", "", "Generate a sample configuration file at the specified path")

	// Mark required flags
	pullCmd.MarkFlagRequired("source")
	pushCmd.MarkFlagRequired("source")
	pushCmd.MarkFlagRequired("target")
}
