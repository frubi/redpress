package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	// List of used for ping probing
	Hosts []string
	// Interval of ping requests sends
	ProbeInterval int
	// Interval of reports generated. This should be dividable by 3600, so
	// that the reports can be aligned by the hour.
	ReportInterval int
}

func LoadConfig(fn string) (*Config, error) {
	buf, err := os.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}

	conf := new(Config)

	err = yaml.Unmarshal(buf, conf)
	if err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %v", err)
	}

	if len(conf.Hosts) == 0 {
		return nil, fmt.Errorf("invalid config: no hosts")
	}
	if conf.ProbeInterval < 1 {
		return nil, fmt.Errorf("invalid config: probe interval %d too low", conf.ProbeInterval)
	}
	if conf.ReportInterval < 60 {
		return nil, fmt.Errorf("invalid config: report interval %d too low", conf.ReportInterval)
	}
	if conf.ReportInterval <= conf.ProbeInterval {
		return nil, fmt.Errorf("invalid config: report interval less than probe interval")
	}

	return conf, err
}
