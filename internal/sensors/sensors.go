package sensors

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maxb-odessa/slog"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/utils"
)

func Start(conf *config.Config, sensChan chan *config.Sensor) error {

	// configure sensors via hwmon kernel subsystem
	if err := hwmonConfig(conf); err != nil {
		return err
	}

	// start sensors
	for _, col := range conf.Columns {
		for _, grp := range col.Groups {
			for _, sens := range grp.Sensors {
				if err := pollSensor(sens, sensChan); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func pollSensor(sens *config.Sensor, sensChan chan *config.Sensor) error {

	// ignore disabled sensors
	if sens.Disabled {
		slog.Info("skipping disabled sensor '%s'", sens.Name)
		return nil
	}

	// some params checking
	if sens.Options.Divider == 0.0 {
		slog.Info("forcing sensor '%s' divider to 1.0", sens.Name)
		sens.Options.Divider = 1.0
	}

	if sens.Options.Poll < 0.5 {
		slog.Info("forcing sensor '%s' poll interval to 1.0", sens.Name)
		sens.Options.Poll = 1.0
	}

	if sens.Widget.Fractions < 0 || sens.Widget.Fractions > 8 {
		slog.Info("forcing sensor '%s' fractions to 0", sens.Name)
		sens.Widget.Fractions = 0
	} else {
		sens.Pvt.FractionsRatio = math.Pow(10, float64(sens.Widget.Fractions))
	}

	if sens.Options.Min > sens.Options.Max {
		slog.Info("forcing sensor '%s' min/max to %f", sens.Name, sens.Options.Max)
		sens.Options.Min = sens.Options.Max
	}

	sens.Name = utils.SafeHTML(sens.Name)

	var err error
	reader := func() {

		if data, err := os.ReadFile(sens.Pvt.Input); err == nil {

			s := strings.TrimSpace(string(data))

			if value, err := strconv.ParseFloat(s, 64); err == nil {

				sens.Pvt.Lock()

				// this senseor is operational
				sens.Pvt.Offline = false
				sens.Pvt.Value = value

				// apply divider if defined
				if sens.Options.Divider != 1.0 {
					sens.Pvt.Value = value / sens.Options.Divider
				}

				// round to fractions if defined
				if sens.Widget.Fractions > 0 {
					sens.Pvt.Value = math.Round(sens.Pvt.Value*sens.Pvt.FractionsRatio) / sens.Pvt.FractionsRatio
				} else {
					sens.Pvt.Value = math.Round(sens.Pvt.Value)
				}
				// calc percents if we can
				if sens.Options.Min != sens.Options.Max {
					sens.Pvt.Percents = sens.Pvt.Value / sens.Options.Max * 100.0
					if sens.Pvt.Percents > 100.0 {
						sens.Pvt.Percents = 100.0
						slog.Warn("Max value for sensor '%s' is too low (%f < %f)", sens.Name, sens.Options.Max, sens.Pvt.Value)
						// TODO auto adjust Max limit?
					}
					sens.Pvt.AntiPercents = 100.0 - sens.Pvt.Percents
				}

				sens.Pvt.Unlock()

				slog.Debug(5, "sensor '%s' value=%f percents=%f", sens.Name, sens.Pvt.Value, sens.Pvt.Percents)
			}
		}

		if err != nil {
			sens.Pvt.Lock()
			sens.Pvt.Offline = true
			sens.Pvt.Unlock()
		}

		select {
		case sensChan <- sens:
		default:
			slog.Debug(1, "sensors queue is full, discarding sensor data")
		}
	}

	// start sensor poller
	go func() {
		interval := time.Duration(sens.Options.Poll)
		ticker := time.NewTicker(interval * time.Second)
		for {
			select {
			case <-ticker.C:
				reader()
			}
		}

	}()

	return nil
}
