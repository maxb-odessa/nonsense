package sensors

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danwakefield/fnmatch"
	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
	"github.com/maxb-odessa/nonsens/internal/utils"

	"github.com/maxb-odessa/slog"
)

const (
	HWMON_PATH = "/sys/class/hwmon"
)

// setup all configured sensors
func setupAllSensors(conf *config.Config) error {

	for _, col := range conf.Columns {
		for _, grp := range col.Groups {
			for _, sens := range grp.Sensors {
				sens.Prepare()
				if SetupSensor(sens) {
					slog.Info("Configured sensor '%s', device '%s'", sens.Options.Input, sens.Options.Device)
				}
			}
			grp.SetId(utils.MakeUID())
		}
	}

	return nil
}

// setup single sensor
func SetupSensor(sens *sensor.Sensor) bool {

	if sens.Options.Device == "" || sens.Options.Input == "" {
		slog.Warn("Ignoring invalid sensor '%s' '%s' %s'", sens.Name, sens.Options.Device, sens.Options.Input)
		return false
	}

	// update sensor dir
	if dir := findSensorDir(sens.Options.Device); dir == "" {
		slog.Warn("Failed to find sensor '%s/%s/%s' dir", sens.Name, sens.Options.Device, sens.Options.Input)
		return false
	} else {
		sens.Runtime.Dir = dir
	}

	// set sensor input file (full path, this is the runtime value)
	sens.SetInput(sens.Runtime.Dir + sens.Options.Input)

	return true
}

// scan /sys/class/hwmon/hwmonX dirs and extract all the sensors found
func ScanAllSensors() *config.Config {

	conf := new(config.Config)

	// make a list of all hwmon devices
	devices := make(map[string][]string, 0)

	dirs, err := os.ReadDir(HWMON_PATH)
	if err != nil {
		slog.Err("scan of '%s' failed: %s", err)
		return nil
	}

	for _, dir := range dirs {
		dirName := HWMON_PATH + "/" + dir.Name() + "/"
		if dev, err := filepath.EvalSymlinks(dirName + "device"); err != nil {
			slog.Warn("Could not resolve '%s': %s", dirName+"device", err)
		} else {
			devName := filepath.Base(dev)
			if files := getDeviceInputs(dirName); len(files) > 0 {
				devices[devName] = files
			}
		}
	}

	// collect all found sensors
	columnIdx := 0
	for device, inputs := range devices {

		dir := findSensorDir(device)

		// make a group
		group := new(config.Group)

		for _, input := range inputs {

			se := new(sensor.Sensor)
			se.Prepare()
			se.SetDefaults()

			se.Runtime.Dir = dir
			se.Options.Device = device
			se.Options.Input = input

			guessSensorOptions(se)

			SetupSensor(se)

			conf.AddSensor(se, group)

		}

		// add non-empty group to 1-st column
		if len(group.Sensors) > 0 {
			group.SetId(utils.MakeUID())
			guessGroupOptions(dir, group)
			conf.AddGroup(columnIdx, group)
			// make new column if current is too long (just for beauty)
			if len(conf.Columns[columnIdx].Groups) >= 4 {
				columnIdx++
			}
		}

	}

	return conf
}

func guessGroupOptions(dir string, gr *config.Group) {

	namePatterns := []string{
		"name",
		"device/manufacturer",
		"device/model_name",
		"device/model",
	}

	// guess group name
	grNames := make([]string, 0)
	for _, file := range namePatterns {
		if name, err := os.ReadFile(dir + file); err == nil {
			grNames = append(grNames, strings.TrimSpace(string(name)))
		}
	}

	if len(grNames) > 0 {
		gr.Name = strings.Join(grNames, "/")
		if len(gr.Name) > 30 {
			gr.Name = gr.Name[0:30] // don't make group name too long
		}
	}

}

func guessSensorOptions(sens *sensor.Sensor) {

	inPrefix := strings.Split(sens.Options.Input, "_")[0]

	// guess sensor name
	if label, err := os.ReadFile(sens.Runtime.Dir + inPrefix + "_label"); err == nil {
		sens.Name = strings.TrimSpace(string(label))
	} else {
		sens.Name = sens.Options.Input
	}

	// guess units
	inName := sens.Options.Input
	if fparts := strings.Split(inName, "/"); len(fparts) > 1 {
		inName = fparts[1]
	}

	if strings.HasPrefix(inName, "fan") {
		sens.Widget.Units = "rpm"
	} else if strings.HasPrefix(inName, "temp") {
		sens.Options.Divider = 1_000.0
		sens.Widget.Units = `&deg;C`
	} else if strings.HasPrefix(inName, "freq") {
		sens.Options.Divider = 1_000_000.0
		sens.Widget.Fractions = 2
		sens.Widget.Units = "MHz"
	} else if strings.HasPrefix(inName, "mem") {
		sens.Options.Divider = 1_000_000.0
		sens.Widget.Fractions = 2
		sens.Widget.Units = "MBytes"
	} else if strings.HasPrefix(inName, "power") {
		sens.Options.Divider = 1_000_000.0
		sens.Widget.Fractions = 1
		sens.Widget.Units = "Watts"
	} else if strings.HasPrefix(inName, "in") {
		sens.Options.Divider = 1_000.0
		sens.Widget.Fractions = 3
		sens.Widget.Units = "Volts"
	} else if strings.HasPrefix(inName, "voltage") {
		sens.Options.Divider = 1_000_000.0
		sens.Widget.Fractions = 3
		sens.Widget.Units = "Volts"
	} else if strings.HasPrefix(inName, "capacity") {
		sens.Widget.Fractions = 1
		sens.Widget.Units = "%"
	} else if strings.HasPrefix(inName, "energy") {
		sens.Options.Divider = 1_000_000.0
		sens.Widget.Fractions = 1
		sens.Widget.Units = "Wh"
	} else {
		sens.Widget.Units = "units"
	}

	// guess sensor min/max value
	if sens.Options.Min == 0.0 && sens.Options.Max == 0.0 {

		minFile := sens.Runtime.Dir + inPrefix + "_min"
		if min, err := os.ReadFile(minFile); err == nil {
			sens.Options.Min, _ = strconv.ParseFloat(strings.TrimSpace(string(min)), 64) // TODO: check error
			slog.Info("Using Min value '%f' for sensor '%s (%s)'", sens.Options.Min, sens.Options.Device, sens.Name)
		}

		maxFile := sens.Runtime.Dir + inPrefix + "_max"
		if max, err := os.ReadFile(maxFile); err == nil {
			sens.Options.Max, _ = strconv.ParseFloat(strings.TrimSpace(string(max)), 64) // TODO: check error
			slog.Info("Using Max values '%f' for sensor '%s (%s)'", sens.Options.Max, sens.Options.Device, sens.Name)
		}

		sens.Options.Min /= sens.Options.Divider
		sens.Options.Max /= sens.Options.Divider

	}

}

func getDeviceInputs(dir string) []string {

	// possible file names that provide sensor data
	inputPatterns := []string{
		"*_in",
		"*_input",
		"*_raw",
		"*capacity",
		"*_now",
		"*_used",
		"*_use",
		"*_average",
		"*_avg",
	}

	inputs := make([]string, 0)

	// extract all sensors within each device dir and 'device' subdir
	files, err := os.ReadDir(dir)
	if err != nil {
		return inputs
	}

	for _, file := range files {

		for _, input := range inputPatterns {
			if fnmatch.Match(input, file.Name(), fnmatch.FNM_PATHNAME) {
				inputs = append(inputs, file.Name())
			}

		}
	}

	files2, err := os.ReadDir(dir + "/device")
	if err != nil {
		return inputs
	}

	for _, file := range files2 {

		for _, input := range inputPatterns {
			if fnmatch.Match(input, file.Name(), fnmatch.FNM_PATHNAME) {
				inputs = append(inputs, "device/"+file.Name())
			}
		}

	}

	return inputs
}

// find device real dir under /sys
func findSensorDir(device string) string {

	dirs, err := os.ReadDir(HWMON_PATH)
	if err != nil {
		slog.Err("scan of '%s' failed: %s", err)
		return ""
	}

	for _, dir := range dirs {

		// is this really our device?
		dirName := HWMON_PATH + "/" + dir.Name() + "/"
		if dev, err := filepath.EvalSymlinks(dirName + "device"); err != nil {
			slog.Warn("Could not resolve '%s': %s", dirName+"device", err)
		} else if filepath.Base(dev) == device {
			return dirName
		}

	}

	return ""
}
