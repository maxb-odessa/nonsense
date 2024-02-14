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

// TODO duplicate code!

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

// find matching sensor and setup it
func setupAllSensors(conf *config.Config) error {

	// find named sensor and setup it
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
		sens.SetInput("")
		return false
	}

	dir := findSensorDir(sens.Options.Device)
	if dir == "" {
		slog.Warn("Failed to find sensor '%s/%s/%s' dir", sens.Name, sens.Options.Device, sens.Options.Input)
		sens.SetInput("")
		return false
	}

	// is this really our device?
	if dev, err := filepath.EvalSymlinks(dir + "device"); err != nil {
		slog.Warn("Could not resolve '%s': %s", dir+"device", err)
		sens.SetInput("")
		return false
	} else {
		// this is not our device
		if filepath.Base(dev) != sens.Options.Device {
			sens.SetInput("")
			return false
		}
	}

	sens.SetInput(dir + sens.Options.Input)

	// default divider
	if sens.Options.Divider == 0.0 {
		sens.Options.Divider = 1.0
	}

	inPrefix := strings.Split(sens.Options.Input, "_")[0]

	// check and read *_min and _max
	if sens.Options.Min == 0.0 && sens.Options.Max == 0.0 {
		minFile := dir + inPrefix + "_min"
		if min, err := os.ReadFile(minFile); err == nil {
			sens.Options.Min, _ = strconv.ParseFloat(strings.TrimSpace(string(min)), 64) // TODO: check error
			sens.Options.Min /= sens.Options.Divider
			slog.Info("Using Min value '%f' for sensor '%s (%s)'", sens.Options.Min, sens.Options.Device, sens.Name)
		}
		maxFile := dir + inPrefix + "_max"
		if max, err := os.ReadFile(maxFile); err == nil {
			sens.Options.Max, _ = strconv.ParseFloat(strings.TrimSpace(string(max)), 64) // TODO: check error
			sens.Options.Max /= sens.Options.Divider
			slog.Info("Using Max values '%f' for sensor '%s (%s)'", sens.Options.Max, sens.Options.Device, sens.Name)
		}
	}

	return true
}

// scan /sys/class/hwmon/hwmonX dirs and extract all the sensors found
func ScanAllSensors() []*config.Column {

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
			if files, err := getDeviceInputs(dirName); err == nil {
				devices[devName] = files
			}
		}
	}

	// setupp all found sensors
	for device, inputs := range devices {

		// make a group

		for _, input := range inputs {

			// make a sensor

			se := new(sensor.Sensor)

			se.Options.Device = device
			se.Options.Input = input

			if SetupSensor(se) {
				se.Prepare()
				// TODO add sensor to group
			}
		}

		// add group to columns if not empty

	}

	return nil
}

func getDeviceInputs(dir string) ([]string, error) {

	inputs := make([]string, 0)

	files, err := os.ReadDir(dir)
	if err != nil {
		return inputs, err
	}

	// possible file names that provide sensor data
	inputPatterns := []string{"*_input", "*_raw", "capacity", "device/capacity"}

	// extract all sensors within each device
	for _, file := range files {
		for _, input := range inputPatterns {
			if fnmatch.Match(input, file.Name(), fnmatch.FNM_PATHNAME) {
				inputs = append(inputs, file.Name())
			}
		}
	}

	return inputs, nil
}
