package main

import (
	"fmt"
	"os"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Version     string `yaml:"version"`
	BinURL      string `yaml:"bin_url"`
	InstallPath string `yaml:"install_path"`
}

func loadConfig() (*Config, error) {
	file, err := os.Open("config.yaml")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &Config{}
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, err
	}

	if config.Version == "" || config.BinURL == "" || config.InstallPath == "" {
		return nil, fmt.Errorf("missing required fields in config")
	}

	return config, nil
}

func main() {
	config, err := loadConfig()
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	fmt.Println("Starting installation for version:", config.Version)
	// Execute installation workflow steps here
}