package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Configuration struct {
	ModulesDirectory string
	DriversDirectory string
	RiemannHost      string
	RiemannProtocol  string
	LogFile          string
	LogLevel         string
	PidFile          string
}

func NewConfiguration() *Configuration {
	return &Configuration{"custom-modules", "drivers", "localhost:5555", "udp", "-", "info", ""}
}

func GetConfiguration(fileName string) (*Configuration, error) {
	file, err := os.Open(fileName)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(file)
	cfg := NewConfiguration()
	err = decoder.Decode(cfg)
	if err != nil {
		return nil, err
	}

	if cfg.RiemannProtocol != "tcp" && cfg.RiemannProtocol != "udp" {
		return nil, errors.New("Bad riemann protocol")
	}

	if cfg.DriversDirectory == "" {
		return nil, errors.New("Empty drivers directory")
	}

	cfg.DriversDirectory, _ = filepath.Abs(cfg.DriversDirectory)

	if cfg.ModulesDirectory != "" {
		cfg.ModulesDirectory, _ = filepath.Abs(cfg.ModulesDirectory)
	}

	return cfg, nil
}
