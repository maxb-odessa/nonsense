package config

import (
	"encoding/json"
	"os"
	"sync"
)

type Sensor struct {
	Priv struct { // runtime data
		sync.Mutex
		Online     bool
		CurrValue  float64
		Percent    float64
		Percent100 float64
	}
	Disabled     bool    `json:"enabled"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Group        string  `json:"group"`
	Source       string  `json:"source"`
	Reopen       bool    `json:"reopen"`
	MinValue     float64 `json:"min value"`
	MaxValue     float64 `json:"max value"`
	Divider      float64 `json:"divider"`
	Units        string  `json:"units"`
	PollInterval float64 `json:"poll interval"`
}

type Config struct {
	Server struct {
		Listen    string `json:"listen"`
		Resources string `json:"resources"`
	} `json:"server"`

	Sensors [][]*Sensor `json:"sensors"`
}

func (c *Config) Load(path string) error {

	if data, err := os.ReadFile(path); err != nil {
		return err
	} else if err := json.Unmarshal(data, c); err != nil {
		return err
	}

	return nil
}
