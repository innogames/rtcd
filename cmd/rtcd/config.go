// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service"

	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Service service.Config
	Logger  logger.Config
}

func (c Config) IsValid() error {
	if err := c.Service.IsValid(); err != nil {
		return err
	}
	if err := c.Logger.IsValid(); err != nil {
		return err
	}
	return nil
}

func (c *Config) SetDefaults() {
	c.Service.API.HTTP.ListenAddress = ":8045"
	c.Service.RTC.ICEPortUDP = 8443
	c.Service.Store.DataSource = "/tmp/rtcd_db"
	c.Logger.EnableConsole = true
	c.Logger.ConsoleJSON = false
	c.Logger.ConsoleLevel = "INFO"
	c.Logger.EnableFile = true
	c.Logger.FileJSON = true
	c.Logger.FileLocation = "rtcd.log"
	c.Logger.FileLevel = "DEBUG"
	c.Logger.EnableColor = false
}

// loadConfig reads the config file and returns a new Config,
// This method overrides values in the file if there is any environment
// variables corresponding to a specific setting.
func loadConfig(path string) (Config, error) {
	var cfg Config
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		log.Printf("config file not found at %s, using defaults", path)
		cfg.SetDefaults()
	} else if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to decode config file: %w", err)
	}
	if err := envconfig.Process("rtcd", &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
