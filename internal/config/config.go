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

	// runtime data
	Priv struct {
		sync.Mutex
		Id             string  // uniq sensor identifier
		Offline        bool    // is offline?
		Value          float64 // current read value
		Percents       float64 // calculated percents (based on Min and Max)
		AntiPercents   float64 // = 100 - percents
		Input          string  // full path to sensor input file, may vary across reboots
		FractionsRatio float64 // calculated franction to be shown
	} `json:"-"`

	// configured data
	Name     string `json:"name"`     // name to show
	Disabled bool   `json:"disabled"` // do not show if true
	Group    string `json:"group"`    // show within this group

	Sensor struct {
		Device  string  `json:"device"`  // device id as in /sys/devices/..., i.e. 0000:09:00.0
		Input   string  `json:"input"`   // short input data file name relative to /sys/class/hwmon/hwmonX/
		Min     float64 `json:"min"`     // min value
		Max     float64 `json:"max"`     // max value
		Divider float64 `json:"divider"` // value divider, i.e. 1000 for temperature values like 42123 which 42.123 deg
		Poll    float64 `json:"poll"`    // poll interval, in seconds
	} `json:"sensor"`

	Widget struct {
		Type      string `json:"type"`      // gauge, static, text, blink, etc (TBD)
		Units     string `json:"units"`     // suffix shown value with units string
		Fractions int    `json:"fractions"` // show only this number of value fractions, i.e. 2 = 1.23 for 1.23456 value
		Color0    string `json:"color0"`    // min value color (at 0%)
		Color100  string `json:"color100"`  //max value color (at 100%)
	} `json:"widget"`
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
