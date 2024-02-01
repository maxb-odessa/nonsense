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

	var conf config.Config

	//defaults
	conf.Server.Listen = ":12345"
	conf.Server.Resources = "$HOME/.local/share/nonsens"

	// no groups, go columns, just plain structure
	conf.Sensors = make([][]*config.Sensor, 4)
	conf.Sensors[0] = make([]*config.Sensor, 0)

	slog.Info("Scanning %s ...", config.HWMON_PATH)

	dirs, err := os.ReadDir(config.HWMON_PATH)
	if err != nil {
		log.Fatal(err)
	}

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
			conf.Sensors[0] = append(conf.Sensors[0], s...)
		}

	}

	b, err := json.MarshalIndent(conf, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))

	//slog.Info("Config: %+v", conf)

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
	s.Sensor.Device = device

	in := strings.Split(input, "_")[0]

	// try to guess device name
	for _, n := range []string{"device/manufacturer", "device/model_name", "device/model"} {
		na := make([]string, 0)
		if name, err := os.ReadFile(dir + n); err == nil {
			na = append(na, strings.TrimSpace(string(name)))
		}
		s.Group = strings.Join(na, " ")
	}

	if s.Group == "" {
		if n, err := os.ReadFile(dir + "name"); err == nil {
			name := strings.TrimSpace(string(n))
			s.Group = name
		}
	}

	s.Group += " (" + device + ")"

	s.Name = in
	if l, err := os.ReadFile(dir + in + "_label"); err == nil {
		s.Name = strings.TrimSpace(string(l)) + "," + s.Name
	}

	// TODO
	if len(in) > 3 && input[0:3] == "fan" {
		s.Widget.Units = "rpm"
	} else if len(input) > 4 && input[0:4] == "temp" {
		s.Widget.Units = `&deg;C`
	} else {
		s.Widget.Units = "units"
	}

	s.Sensor.Input = input
	s.Sensor.Divider = 1.0
	s.Sensor.Poll = 1.0

	if min, err := os.ReadFile(dir + in + "_min"); err == nil {
		s.Sensor.Min, _ = strconv.ParseFloat(strings.TrimSpace(string(min)), 64)
	}
	if max, err := os.ReadFile(dir + in + "_max"); err == nil {
		s.Sensor.Max, _ = strconv.ParseFloat(strings.TrimSpace(string(max)), 64)
	}

	s.Widget.Color0 = "#00FF00"
	s.Widget.Color100 = "#FF0000"

	return s
}
