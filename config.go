package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
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

	// Use an unprivileged socket
	Unprivileged bool

	// Pointer to InfluxDB configuration block. This block may be empty, if
	// no export to InfluxDB is wanted.
	Influxdb *Influx
}

type Influx struct {
	// URL of InfluxDB instance
	Url string

	// Attributes used for storing the values
	Bucket string
	Token  string
	Org    string
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

	if conf.Influxdb != nil {
		if conf.Influxdb.Url == "" {
			return nil, fmt.Errorf("invalid config: InfluxDB url missing")
		}
		if conf.Influxdb.Bucket == "" {
			return nil, fmt.Errorf("invalid config: InfluxDB bucket missing")
		}
		if conf.Influxdb.Token == "" {
			return nil, fmt.Errorf("invalid config: InfluxDB token missing")
		}
		if conf.Influxdb.Org == "" {
			return nil, fmt.Errorf("invalid config: InfluxDB org missing")
		}
	}

	return conf, err
}
