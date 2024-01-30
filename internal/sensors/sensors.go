package sensors

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maxb-odessa/slog"

	"github.com/maxb-odessa/nonsens/internal/config"
)

func Start(conf *config.Config, sensChan chan *config.Sensor) error {

	// configure sensors via hwmon kernel subsystem
	if err := hwmonConfig(conf.Sensors); err != nil {
		return err
	}

	// start sensors
	for _, group := range conf.Sensors {
		for _, sens := range group {
			if err := sensor(sens, sensChan); err != nil {
				return err
			}
		}
	}

	return nil
}

func sensor(sens *config.Sensor, sensChan chan *config.Sensor) error {

	// ignore disabled sensors
	if sens.Disabled {
		slog.Info("skipping disabled sensor '%s'", sens.Name)
		return nil
	}

	// some params checking
	if sens.Sensor.Divider == 0.0 {
		slog.Info("forcing sensor '%s' divider to 1.0", sens.Name)
		sens.Sensor.Divider = 1.0
	}

	if sens.Sensor.Poll < 0.01 {
		slog.Info("forcing sensor '%s' poll interval to 1.0", sens.Name)
		sens.Sensor.Poll = 1.0
	}

	if sens.Widget.Fractions < 0 || sens.Widget.Fractions > 8 {
		slog.Info("forcing sensor '%s' fractions to 0", sens.Name)
		sens.Widget.Fractions = 0
	} else {
		sens.Priv.FractionsRatio = math.Pow(10, float64(sens.Widget.Fractions))
	}

	if sens.Sensor.Min > sens.Sensor.Max {
		slog.Info("forcing sensor '%s' min/max to %f", sens.Name, sens.Sensor.Max)
		sens.Sensor.Min = sens.Sensor.Max
	}

	var err error
	reader := func() {

		if data, err := os.ReadFile(sens.Priv.Input); err == nil {

			s := strings.TrimSpace(string(data))

			if value, err := strconv.ParseFloat(s, 64); err == nil {

				sens.Priv.Lock()

				// this senseor is operational
				sens.Priv.Offline = false
				sens.Priv.Value = value

				// apply divider if defined
				if sens.Sensor.Divider != 1.0 {
					sens.Priv.Value = value / sens.Sensor.Divider
				}

				// round to fractions if defined
				if sens.Widget.Fractions > 0 {
					sens.Priv.Value = math.Round(sens.Priv.Value*sens.Priv.FractionsRatio) / sens.Priv.FractionsRatio
				} else {
					sens.Priv.Value = math.Round(sens.Priv.Value)
				}
				// calc percents if we can
				if sens.Sensor.Min != sens.Sensor.Max {
					sens.Priv.Percents = sens.Priv.Value / sens.Sensor.Max * 100.0
					if sens.Priv.Percents > 100.0 {
						sens.Priv.Percents = 100.0
						slog.Warn("Max value for sensor '%s' is too low (%f < %f)", sens.Name, sens.Sensor.Max, sens.Priv.Value)
						// TODO auto adjust Max limit?
					}
					sens.Priv.AntiPercents = 100.0 - sens.Priv.Percents
				}

				sens.Priv.Unlock()

				slog.Debug(1, "sensor '%s' value=%f percents=%f", sens.Name, sens.Priv.Value, sens.Priv.Percents)
			}
		}

		if err != nil {
			sens.Priv.Lock()
			sens.Priv.Offline = true
			sens.Priv.Unlock()
		}

		select {
		case sensChan <- sens:
		default:
			slog.Debug(1, "sensors queue is fuill, discarding sensor data")
		}
	}

	// start sensor poller
	go func() {
		interval := time.Duration(sens.Sensor.Poll)
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
