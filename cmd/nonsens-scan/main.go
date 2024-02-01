package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danwakefield/fnmatch"
	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/utils"

	"github.com/maxb-odessa/slog"
)

func main() {

	conf := new(config.Config)

	//defaults
	conf.Server = &config.Server{
		Listen:         ":12345",
		Resources:      "$HOME/.local/share/nonsens",
		ConfigOverride: "$HOME/.config/nonsens/override.json",
	}

	slog.Info("Scanning %s ...", config.HWMON_PATH)

	dirs, err := os.ReadDir(config.HWMON_PATH)
	if err != nil {
		log.Fatal(err)
	}

	// 1-st stage: find all sensors and put them here
	sensors := make([]*config.Sensor, 0)

	for _, dir := range dirs {

		if !fnmatch.Match("hwmon*", dir.Name(), fnmatch.FNM_PATHNAME) {
			continue
		}

		path, _ := filepath.EvalSymlinks(config.HWMON_PATH + "/" + dir.Name())
		if !utils.IsDir(path) {
			continue
		}

		dev, _ := filepath.EvalSymlinks(path + "/device")
		device := filepath.Base(dev)

		s := setupAllSensors(config.HWMON_PATH+"/"+dir.Name()+"/", device)
		if s != nil {
			sensors = append(sensors, s...)
		}

	}

	// 2-d stage: arrange all sensors
	conf.Columns = make([]*config.Column, 0)

	col := 0
	var column *config.Column
	for i, sens := range sensors {

		if i%8 == 0 && col < config.MAX_COLUMNS {
			col++
			column = new(config.Column)
			column.Groups = make([]*config.Group, 0)
			conf.Columns = append(conf.Columns, column)
		}

		g := &config.Group{
			Name:    "Group: " + sens.Name,
			Sensors: make([]*config.Sensor, 0),
		}

		g.Sensors = append(g.Sensors, sens)
		column.Groups = append(column.Groups, g)
	}

	j, _ := json.MarshalIndent(conf, "", "    ")
	fmt.Println(string(j))
}

func setupAllSensors(dir, device string) []*config.Sensor {

	ss := make([]*config.Sensor, 0)

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	// search for _input files
	for _, input := range files {
		if !fnmatch.Match("*_input", input.Name(), fnmatch.FNM_PATHNAME) {
			continue
		}
		in := input.Name()
		if s := setupSensor(dir, device, in); s != nil {
			ss = append(ss, s)
		}

	}

	// search for osther files
	files2, err := os.ReadDir(dir + "device")
	for _, input := range files2 {
		if !fnmatch.Match("capacity", input.Name(), fnmatch.FNM_PATHNAME) {
			continue
		}
		in := input.Name()
		if s := setupSensor(dir, device, "device/"+in); s != nil {
			ss = append(ss, s)
		}

	}

	return ss
}

func setupSensor(dir, device, input string) *config.Sensor {
	s := new(config.Sensor)
	s.Options.Device = device

	in := strings.Split(input, "_")[0]

	if n, err := os.ReadFile(dir + "name"); err == nil {
		name := strings.TrimSpace(string(n))
		if name != "" {
			s.Name = name
		}
	}

	s.Name += "/" + in

	for _, n := range []string{"device/manufacturer", "device/model_name", "device/model"} {
		na := make([]string, 0)
		if name, err := os.ReadFile(dir + n); err == nil {
			na = append(na, strings.TrimSpace(string(name)))
		}
		if len(na) > 0 {
			s.Name += "/" + strings.Join(na, "/")
		}
	}

	if l, err := os.ReadFile(dir + in + "_label"); err == nil {
		s.Name += "/" + strings.TrimSpace(string(l))
	}

	s.Options.Input = input
	s.Options.Divider = 1.0
	s.Options.Poll = 1.0

	if min, err := os.ReadFile(dir + in + "_min"); err == nil {
		s.Options.Min, _ = strconv.ParseFloat(strings.TrimSpace(string(min)), 64)
	}
	if max, err := os.ReadFile(dir + in + "_max"); err == nil {
		s.Options.Max, _ = strconv.ParseFloat(strings.TrimSpace(string(max)), 64)
	}

	// TODO
	if len(in) > 3 && input[0:3] == "fan" {
		s.Widget.Units = "rpm"
	} else if len(input) > 4 && input[0:4] == "temp" {
		s.Widget.Units = `&deg;C`
	} else {
		s.Widget.Units = "units"
	}

	s.Widget.Type = "gauge"
	s.Widget.Fractions = 1
	s.Widget.Color = "#FFFFFF"
	s.Widget.Color0 = "#00FF00"
	s.Widget.Color100 = "#FF0000"

	return s
}
