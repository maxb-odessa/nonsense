package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
	"github.com/maxb-odessa/slog"
)

var configFile string

type Server struct {
	Listen    string `json:"listen"`    // listen to http requests here
	Resources string `json:"resources"` // path to resources dir
}

type Group struct {
	id       string
	Name     string           `json:"name"`
	Disabled bool             `json:"disabled"`
	Sensors  []*sensor.Sensor `json:"sensors"`
}

func (g *Group) Id() string {
	return g.id
}

func (g *Group) SetId(id string) {
	g.id = id
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

	configFile = path

	return nil
}

func (c *Config) Save() error {

	t := time.Now()
	oldConfigFile := configFile + "-" + t.Format("20060102150405")

	slog.Info("Moving config file '%s' to '%s'", configFile, oldConfigFile)
	if err := os.Rename(configFile, oldConfigFile); err != nil {
		return err
	}

	js, _ := json.MarshalIndent(c, "", "    ")

	slog.Info("Saving new config to '%s'", configFile)
	if err := os.WriteFile(configFile, js, 0644); err != nil {
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
			if grp.Id() == id {
				return ci, gi, grp
			}
		}
	}

	return 0, 0, nil
}

func (c *Config) RemoveGroup(g *Group) {
	for _, col := range c.Columns {
		for gi, grp := range col.Groups {
			if grp != g {
				continue
			}
			col.Groups = append(col.Groups[:gi], col.Groups[gi+1:]...)
		}
	}
}

func (c *Config) AddColumn() {
	col := new(Column)
	col.Groups = make([]*Group, 0)
	c.Columns = append(c.Columns, col)
}

func (c *Config) AddGroup(ci int, gr *Group) {
	c.Columns[ci].Groups = append(c.Columns[ci].Groups, gr)
}

// remove empty columns
func (c *Config) Sanitize() {
	for i := 0; i < len(c.Columns); i++ {
		if len(c.Columns[i].Groups) == 0 {
			c.Columns = append(c.Columns[:i], c.Columns[i+1:]...)
		}
	}
}

func (c *Config) MoveGroupToTop(id string) bool {

	ci, gi, gr := c.FindGroupById(id)

	// already at top or not found
	if gi == 0 || gr == nil {
		return false
	}

	for i := gi; i > 0; i-- {
		c.Columns[ci].Groups[i] = c.Columns[ci].Groups[i-1]
	}
	c.Columns[ci].Groups[0] = gr

	return true
}
