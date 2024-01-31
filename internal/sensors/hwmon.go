package sensors

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/utils"

	"github.com/maxb-odessa/slog"
)

// scan hwmon dirs, read required files, config appropriate sensors
func hwmonConfig(sensors [][]*config.Sensor) error {

	// dirty trik, but this is much easier than call WalkDir()
	for i := 0; i < 100; i++ {

		// compose dir path
		dirName := fmt.Sprintf("%s/hwmon%d/", config.HWMON_PATH, i)

		// stop if dir doesn't exist
		if !utils.IsDir(dirName) {
			slog.Debug(9, "'%s' is not a dir", dirName)
			break
		}

		// setup sensor with data, it is ok if we fail
		if err := setupSensors(sensors, dirName); err != nil {
			slog.Warn("Failed to read '%s': %s", dirName, err)
		}

	}

	return nil
}

// find matching sensor and setup it
func setupSensors(sensors [][]*config.Sensor, dir string) error {

	// find named sensor and setup it
	for _, groups := range sensors {
		for _, sens := range groups {
			if setupSingleSensor(sens, dir) {
				slog.Info("Configured sensor '%s' for device '%s'", sens.Name, sens.Sensor.Device)
			} else {
				// make uniq sens ID to address it within html page
				sens.Priv.Id = base64.StdEncoding.EncodeToString([]byte(sens.Sensor.Device + sens.Sensor.Input))
			}
		}
	}

	return nil
}

// setup single sensor
func setupSingleSensor(sens *config.Sensor, dir string) bool {

	// is this really our device?
	if dev, err := filepath.EvalSymlinks(dir + "device"); err != nil {
		slog.Warn("Could not resolve '%s': %s", dir+"device", err)
		return false
	} else {
		// this is not our device
		if filepath.Base(dev) != sens.Sensor.Device {
			return false
		}
	}

	// full path to input data file
	sens.Priv.Input = dir + sens.Sensor.Input

	// default divider
	if sens.Sensor.Divider == 0.0 {
		sens.Sensor.Divider = 1.0
	}

	inPrefix := strings.Split(sens.Sensor.Input, "_")[0]

	// check and read *_min and _max
	if sens.Sensor.Min == 0.0 && sens.Sensor.Max == 0.0 {
		minFile := dir + inPrefix + "_min"
		if min, err := os.ReadFile(minFile); err == nil {
			sens.Sensor.Min, _ = strconv.ParseFloat(strings.TrimSpace(string(min)), 64) // TODO: check error
			sens.Sensor.Min /= sens.Sensor.Divider
			slog.Info("Using Min value '%f' for sensor '%s (%s)'", sens.Sensor.Min, sens.Sensor.Device, sens.Name)
		}
		maxFile := dir + inPrefix + "_max"
		if max, err := os.ReadFile(maxFile); err == nil {
			sens.Sensor.Max, _ = strconv.ParseFloat(strings.TrimSpace(string(max)), 64) // TODO: check error
			sens.Sensor.Max /= sens.Sensor.Divider
			slog.Info("Using Max values '%f' for sensor '%s (%s)'", sens.Sensor.Max, sens.Sensor.Device, sens.Name)
		}
	}

	return true
}
