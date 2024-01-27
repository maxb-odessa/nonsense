package sensors

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/slog"
)

// scan hwmon dirs, read required files, config appropriate sensors
func hwmonConfig(sensors [][]*config.Sensor) error {

	// dirty trik, but this is much easier than call WalkDir()
	for i := 0; i < 100; i++ {

		// compose dir path
		dirName := fmt.Sprintf("%s/hwmon%d/", config.HWMON_PATH, i)

		// stop if dir doesn't exist
		if !isDir(dirName) {
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

// return true if dir exists and is accessible
func isDir(dir string) bool {
	if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
		return true
	}
	return false
}

// find matching sensor and setup it
func setupSensors(sensors [][]*config.Sensor, dir string) error {

	// find named sensor and setup it
	for _, groups := range sensors {
		for _, sens := range groups {
			if setupSingleSensor(sens, dir) {
				slog.Info("Configured sensor: Name:[%s] Dev:[%s] Sens:[%s]", sens.Name, sens.Device, sens.Sensor)
			} else {
				// make uniq sens IDto address it within html page
				sens.Priv.Id = sens.Name + sens.Device + sens.Sensor
			}
		}
	}

	return nil
}

// setup single sensor
func setupSingleSensor(sens *config.Sensor, dir string) bool {

	// is this really our device?
	if sens.Device != "" {
		if dev, err := filepath.EvalSymlinks(dir + "device"); err != nil {
			slog.Warn("Could not resolve '%s': %s", dir+"device", err)
			return false
		} else {
			// this is not our device
			device := filepath.Base(dev)
			if filepath.Base(dev) != sens.Device {
				return false
			}
			// save device name
			sens.Device = device
			slog.Info("Using device '%s' for sensor '%s (%s)'", sens.Device, sens.Name, sens.Sensor)
		}
	}

	// check and read *_input
	// mandatory! or disable the sensor until next rescan
	inputPath := dir + sens.Sensor + "_input"
	if _, err := os.ReadFile(inputPath); err != nil {
		//slog.Info("Disabling non-existing sensor '%s %s (%s)'", sens.Name, sens.Device, sens.Sensor)
		sens.Disabled = true
		return false
	} else {
		sens.Disabled = false
		sens.Priv.InputPath = inputPath
	}

	// check and read *_label
	// if Name is not set - use data from *_label file
	// if failed - use `label (Device)` as a name
	if sens.Name == "" {
		labelFile := dir + sens.Sensor + "_label"
		if label, err := os.ReadFile(labelFile); err == nil {
			sens.Name = strings.TrimSpace(string(label))
		} else {
			sens.Name = sens.Sensor + " (" + sens.Device + ")"
			slog.Info("Using name '%s' for unnamed sensor", sens.Name)
		}
	}

	// check and read *_min if requested
	if sens.MinAuto {
		minFile := dir + sens.Sensor + "_min"
		if min, err := os.ReadFile(minFile); err == nil {
			sens.Min, _ = strconv.ParseFloat(strings.TrimSpace(string(min)), 64) // TODO: check error
			if sens.Divider != 0.0 {
				sens.Min = sens.Min / sens.Divider
			}
			slog.Info("Using MIN value '%f' for sensor '%s (%s)'", sens.Min, sens.Device, sens.Sensor)
		}
	}

	// check and read *_man if requested
	if sens.MaxAuto {
		maxFile := dir + sens.Sensor + "_max"
		if max, err := os.ReadFile(maxFile); err == nil {
			sens.Max, _ = strconv.ParseFloat(strings.TrimSpace(string(max)), 64) // TODO: check error
			if sens.Divider != 0.0 {
				sens.Max = sens.Max / sens.Divider
			}
			slog.Info("Using MAX value '%f' for sensor '%s (%s)'", sens.Max, sens.Device, sens.Sensor)
		}
	}

	return true
}
