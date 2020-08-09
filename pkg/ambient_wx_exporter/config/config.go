package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

// GaugeConfig defines a gaugej
type GaugeConfig struct {
	APIName string            `json:"api_name"`
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels"`
}

// DeviceConfig is the configuration for a single device
type DeviceConfig struct {
	MacAddress string `json:"mac_address"`
	Gauges     []GaugeConfig
}

// Config is the format of the JSON config file
type Config struct {
	Devices []DeviceConfig
}

// Read a json config file
func Read(configFile string, config *Config) error {
	file, err := os.Open(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	bytes, _ := ioutil.ReadAll(file)
	jsonError := json.Unmarshal(bytes, config)
	if jsonError != nil {
		return jsonError
	}
	return nil
}
