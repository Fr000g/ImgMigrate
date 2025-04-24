package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Registry  *RegistryConfig `yaml:"registry,omitempty"`
	ImageTask []ImageTask     `yaml:"images"`
}

// RegistryConfig contains registry authentication information
type RegistryConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Insecure bool   `yaml:"insecure,omitempty"`
}

// ImageTask represents a single image processing task
type ImageTask struct {
	Source           string   `yaml:"source"`
	Target           string   `yaml:"target,omitempty"`
	Architectures    []string `yaml:"architectures,omitempty"`
	AllArchitecture  bool     `yaml:"all_architectures,omitempty"`
	SaveOptions      `yaml:",inline"`
	OperatingSystems []string `yaml:"operating_systems,omitempty"`
	CreateMultiArch  bool     `yaml:"create_multi_arch,omitempty"`
}

// SaveOptions contains options for saving images
type SaveOptions struct {
	Save      bool   `yaml:"save,omitempty"`
	OutputDir string `yaml:"output_dir,omitempty"`
	Compress  bool   `yaml:"compress,omitempty"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %v", err)
	}

	return &config, nil
}

// GenerateSampleConfig generates a sample YAML configuration
func GenerateSampleConfig(filename string) error {
	config := Config{
		Registry: &RegistryConfig{
			URL:      "registry.example.com",
			Username: "username",
			Password: "password",
			Insecure: false,
		},
		ImageTask: []ImageTask{
			{
				Source: "nginx:latest",
				SaveOptions: SaveOptions{
					Save:      true,
					OutputDir: "./output",
					Compress:  true,
				},
				AllArchitecture:  true,
				OperatingSystems: []string{"linux"},
				CreateMultiArch:  true,
			},
			{
				Source:           "ubuntu:latest",
				Target:           "registry.example.com/ubuntu:v1",
				Architectures:    []string{"amd64", "arm64"},
				OperatingSystems: []string{"linux"},
				CreateMultiArch:  true,
				SaveOptions: SaveOptions{
					Save: false,
				},
			},
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshaling config: %v", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %v", err)
	}

	fmt.Printf("Sample configuration written to %s\n", filename)
	return nil
}
