package config

import (
	"encoding/json"
	"os"

	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
)

type Server struct {
	Listen    string `json:"listen"`    // listen to http requests here
	Resources string `json:"resources"` // path to resources dir
}

type Group struct {
	Id      string           `json:"id"`
	Name    string           `json:"name"`
	Sensors []*sensor.Sensor `json:"sensors"`
}

type Column struct {
	Groups []*Group `json:"groups"`
}

type Config struct {
	Server  *Server   `json:"server"`  // server config
	Columns []*Column `json:"columns"` // sensors config: columns->groups->sensors
}

func (c *Config) Load(path string) error {

	if data, err := os.ReadFile(path); err != nil {
		return err
	} else if err := json.Unmarshal(data, c); err != nil {
		return err
	}

	return nil
}

func (c *Config) AllSensors() []*sensor.Sensor {

	sarr := make([]*sensor.Sensor, 0)

	for _, c := range c.Columns {
		for _, g := range c.Groups {
			for _, s := range g.Sensors {
				sarr = append(sarr, s)
			}
		}
	}

	return sarr
}

func (c *Config) FindSensorById(id string) *sensor.Sensor {

	for _, c := range c.Columns {
		for _, g := range c.Groups {
			for _, s := range g.Sensors {
				if s.Id() == id {
					return s
				}
			}
		}
	}

	return nil
}

func (c *Config) FindGroupById(id string) (int, int, *Group) {

	for ci, col := range c.Columns {
		for gi, grp := range col.Groups {
			if grp.Id == id {
				return ci, gi, grp
			}
		}
	}

	return 0, 0, nil
}

func (c *Config) RemoveGroup(g *Group) {

	for _, col := range c.Columns {
		for gi, grp := range col.Groups {

			if grp != gr {
				continue
			}

			col.Groups = append(col.Groups[:gi], col.Groups[gi+1:]...)

		}
	}
}
