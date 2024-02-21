package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
	"github.com/maxb-odessa/nonsens/internal/utils"
	"github.com/maxb-odessa/slog"
)

var configFile string

type Server struct {
	Listen    string `json:"listen"`    // listen to http requests here
	Resources string `json:"resources"` // path to resources dir
}

type Group struct {
	id      string
	Name    string           `json:"name"`
	Sensors []*sensor.Sensor `json:"sensors"`
}

func (g *Group) Id() string {
	return g.id
}

func (g *Group) SetId(id string) {
	g.id = id
}

func (g *Group) SetName(name string) {
	g.Name = name
}

type Column struct {
	Groups []*Group `json:"groups"`
}

type Config struct {
	Server      *Server   `json:"server"`       // server config
	SysinfoPoll int       `json:"sysinfo poll"` // sysinfo ticket poll interval
	Columns     []*Column `json:"columns"`      // sensors config: columns->groups->sensors
}

func (c *Config) Load(path string) error {

	if data, err := os.ReadFile(path); err != nil {
		return err
	} else if err := json.Unmarshal(data, c); err != nil {
		return err
	}

	if c.Server == nil {
		return fmt.Errorf("Missing 'server' section in config file")
	}

	configFile = path

	return nil
}

func (c *Config) ImportServerData(c2 *Config) {
	c.Server = c2.Server
	c.SysinfoPoll = c2.SysinfoPoll
}

func (c *Config) Save() error {

	slog.Info("Moving config file '%[1]s' to '%[1]s-'", configFile)
	if err := os.Rename(configFile, configFile+"-"); err != nil {
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

func (c *Config) FindSensorById(id string) (*Group, *sensor.Sensor) {

	for _, c := range c.Columns {
		for _, g := range c.Groups {
			for _, s := range g.Sensors {
				if s.Id() == id {
					return g, s
				}
			}
		}
	}

	return nil, nil
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
			if grp == g {
				col.Groups = append(col.Groups[:gi], col.Groups[gi+1:]...)
				break
			}
		}
	}
}

func (c *Config) AddColumn() {
	col := new(Column)
	col.Groups = make([]*Group, 0)
	c.Columns = append(c.Columns, col)
}

func (c *Config) AddGroup(ci int, gr *Group) {
	if len(c.Columns) <= ci {
		c.AddColumn()
		ci = len(c.Columns) - 1 // to prevent overflow (i.e. column #7 was request when we have only 2)
	}
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

func (c *Config) MoveGroupToTop(gid string) bool {

	ci, gi, gr := c.FindGroupById(gid)

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

func (c *Config) MoveSensorToGroup(se *sensor.Sensor, oldGr *Group, newGid string) {
	var ci, gi int
	var grp *Group

	// add to new group
	ci, gi, grp = c.FindGroupById(newGid)
	if grp == nil {
		return
	}
	c.Columns[ci].Groups[gi].Sensors = append(c.Columns[ci].Groups[gi].Sensors, se)

	// remove from old group
	ci, gi, grp = c.FindGroupById(oldGr.Id())
	for si, s := range c.Columns[ci].Groups[gi].Sensors {
		if s.Id() == se.Id() {
			c.Columns[ci].Groups[gi].Sensors = append(c.Columns[ci].Groups[gi].Sensors[:si], c.Columns[ci].Groups[gi].Sensors[si+1:]...)
			break
		}
	}
}

func (c *Config) RemoveSensor(s *sensor.Sensor) {

	for ci, col := range c.Columns {
		for gi, grp := range col.Groups {
			for si, sen := range grp.Sensors {
				if sen == s {
					c.Columns[ci].Groups[gi].Sensors = append(c.Columns[ci].Groups[gi].Sensors[:si], c.Columns[ci].Groups[gi].Sensors[si+1:]...)
					break
				}
			}
		}
	}
}

func (c *Config) MoveSensorToGroupTop(s *sensor.Sensor) bool {

	for ci, col := range c.Columns {
		for gi, grp := range col.Groups {
			for si, sen := range grp.Sensors {
				if sen == s {
					if si == 0 {
						return false
					}
					for i := si; i > 0; i-- {
						c.Columns[ci].Groups[gi].Sensors[i] = c.Columns[ci].Groups[gi].Sensors[i-1]
					}
					c.Columns[ci].Groups[gi].Sensors[0] = s
					return true
				}
			}
		}
	}

	return false
}

func (c *Config) AddSensor(se *sensor.Sensor, gr *Group) {
	var g *Group

	// make new group if not specified
	if gr == nil {
		g = new(Group)
		g.SetId(utils.MakeUID())
		g.SetName(se.Name)
		g.Sensors = make([]*sensor.Sensor, 0)
		c.AddGroup(0, g) // add new group to column 0
	} else {
		g = gr
	}

	g.Sensors = append(g.Sensors, se)
}
