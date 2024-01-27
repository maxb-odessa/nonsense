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
	if sens.Divider == 0.0 {
		slog.Info("forcing sensor '%s' divider to 1.0", sens.Name)
		sens.Divider = 1.0
	}

	if sens.PollInterval < 0.01 {
		slog.Info("forcing sensor '%s' poll interval to 1.0", sens.Name)
		sens.PollInterval = 1.0
	}

	if sens.Fractions < 0 || sens.Fractions > 8 {
		slog.Info("forcing sensor '%s' fractions to 0", sens.Name)
		sens.Fractions = 0
	} else {
		sens.Priv.FractionsRatio = math.Pow(10, float64(sens.Fractions))
	}

	var err error
	reader := func() {

		if data, err := os.ReadFile(sens.Priv.InputPath); err == nil {

			s := strings.TrimSpace(string(data))

			if value, err := strconv.ParseFloat(s, 64); err == nil {

				sens.Priv.Lock()

				// this senseor is operational
				sens.Priv.Online = true
				sens.Priv.Value = value

				// apply divider if defined
				if sens.Divider != 1.0 {
					sens.Priv.Value = value / sens.Divider
				}

				// round to fractions if defined
				if sens.Fractions > 0 {
					sens.Priv.Value = math.Round(sens.Priv.Value*sens.Priv.FractionsRatio) / sens.Priv.FractionsRatio
				} else {
					sens.Priv.Value = math.Round(sens.Priv.Value)
				}
				// calc percents if we can
				if sens.Percents {
					sens.Priv.Percent = sens.Priv.Value / sens.Max * 100.0
					if sens.Priv.Percent > 100.0 {
						sens.Priv.Percent = 100.0
						slog.Warn("Max value for sensor '%s' is too low (%f < %f)", sens.Name, sens.Max, sens.Priv.Value)
					}
					sens.Priv.Percent100 = 100.0 - sens.Priv.Percent
				}

				sens.Priv.Unlock()

				slog.Debug(1, "sensor '%s' value=%f percent=%f (%v)", sens.Name, sens.Priv.Value, sens.Priv.Percent, sens.Percents)
			}
		}

		if err != nil {
			sens.Priv.Lock()
			sens.Priv.Online = false
			sens.Priv.Unlock()
		}

		select {
		case sensChan <- sens:
		default:
			slog.Debug(1, "sensors queue is fuill, discarding sensor data")
		}
	}

	// start sensor poller
	// TODO use notify! DO NOT POLL !!!!!!!!!!
	go func() {
		interval := time.Duration(math.Round(sens.PollInterval))
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
