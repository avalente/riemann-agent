package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func normalizePath(configFile string, fileName string) string {
	if strings.HasPrefix(fileName, "./") {
		cfgDir := filepath.Dir(configFile)
		fileName = filepath.Join(cfgDir, fileName[2:])
	}
	fileName, _ = filepath.Abs(fileName)
	return fileName
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

	cfg.DriversDirectory = normalizePath(fileName, cfg.DriversDirectory)

	if cfg.ModulesDirectory != "" {
		cfg.ModulesDirectory = normalizePath(fileName, cfg.ModulesDirectory)
	}

	return cfg, nil
}
