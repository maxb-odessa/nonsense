package config

import (
	"encoding/json"
	"os"
	"sync"
)

const (
	HWMON_PATH = "/sys/class/hwmon"
)

type Sensor struct {
	Priv struct { // runtime data
		sync.Mutex
		Online         bool
		Value          float64
		Percent        float64
		Percent100     float64
		InputPath      string
		FractionsRatio float64
	}
	Disabled     bool    `json:"disabled"`
	Name         string  `json:"name"`
	Group        string  `json:"group"`
	Device       string  `json:"device"`
	Sensor       string  `json:"sensor"`
	Min          float64 `json:"min"`
	MinAuto      bool    `json:"min auto"`
	Max          float64 `json:"max"`
	MaxAuto      bool    `json:"max auto"`
	Divider      float64 `json:"divider"`
	Units        string  `json:"units"`
	Fractions    int     `json:"fractions"`
	Percents     bool    `json:"percents"`
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
