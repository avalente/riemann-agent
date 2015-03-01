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

func getConfiguration(fileName string) (*Configuration, error) {
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
	} else {
		cfg.DriversDirectory, err = filepath.Abs(cfg.DriversDirectory)
		if err != nil {
			return nil, errors.New("Can't get absolute path for drivers directory")
		}
	}

	if cfg.ModulesDirectory != "" {
		cfg.ModulesDirectory, err = filepath.Abs(cfg.ModulesDirectory)
		if err != nil {
			return nil, errors.New("Can't get absolute path for modules directory")
		}
	}

	return cfg, nil
}
