package config

import (
	"encoding/json"
	"os"
	"sync"
)

const (
	HWMON_PATH = "/sys/class/hwmon"
)

// runtime data, not for save
type SensorPrivate struct {
	sync.Mutex
	Id             string  // uniq sensor identifier
	Offline        bool    // is offline?
	Value          float64 // current read value
	Percents       float64 // calculated percents (based on Min and Max)
	AntiPercents   float64 // = (100 - percents) used for gauges
	Input          string  // full path to sensor input file, may vary across reboots
	FractionsRatio float64 // calculated fractions ratio to be shown
}

// config data read from file
type Sensor struct {
	Private *SensorPrivate `json:"-"`

	// configured data
	Name     string `json:"name"`     // name to show
	Disabled bool   `json:"disabled"` // do not show if true

	Options struct {
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
		Color     string `json:"color"`     // text color
		Color0    string `json:"color0"`    // min value color (at 0%)
		Color100  string `json:"color100"`  // max value color (at 100%)
	} `json:"widget"`
}

type Group struct {
	Name    string    `json:"name"`
	Sensors []*Sensor `json:"sensors"`
}

type Column struct {
	Groups []*Group `json:"groups"`
}

type Server struct {
	Listen    string `json:"listen"`
	Resources string `json:"resources"`
}

type Config struct {
	Server  *Server   `json:"server"`
	Columns []*Column `json:"columns"`
}

const MAX_COLUMNS = 10

func (c *Config) Load(path string) error {

	if data, err := os.ReadFile(path); err != nil {
		return err
	} else if err := json.Unmarshal(data, c); err != nil {
		return err
	}

	return nil
}
