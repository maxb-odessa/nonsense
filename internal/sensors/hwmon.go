package sensors

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
	"github.com/maxb-odessa/nonsens/internal/utils"

	"github.com/maxb-odessa/slog"
)

const (
	HWMON_PATH = "/sys/class/hwmon"
)

// find device real dir under /sys
func findSensorDir(device string) string {

	// dirty trick, but this is much easier than call WalkDir()
	for i := 0; i < 100; i++ {

		// compose dir path
		dirName := fmt.Sprintf("%s/hwmon%d/", HWMON_PATH, i)

		// stop if dir doesn't exist
		if !utils.IsDir(dirName) {
			break
		}

		// is this really our device?
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
			grp.SetId(fmt.Sprintf("%x", md5.Sum([]byte(time.Now().String()))))
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
		slog.Warn("Failed to find sensor '%s/%/%s' dir", sens.Name, sens.Options.Device, sens.Options.Input)
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
