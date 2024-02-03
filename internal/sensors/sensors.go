package sensors

import (
	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
	"github.com/maxb-odessa/slog"
)

var sensChan chan *sensor.Sensor

func Chan() chan *sensor.Sensor {
	return sensChan
}

func Run(conf *config.Config) error {

	// configure sensors via hwmon kernel subsystem
	if err := hwmonConfig(conf); err != nil {
		return err
	}

	sensChan = make(chan *sensor.Sensor, 64)

	// start sensors
	for _, sens := range conf.AllSensors() {

		// ignore disabled sensors
		if sens.Disabled {
			slog.Info("skipping disabled sensor '%s'", sens.Name)
			continue
		}

		if err := sens.Start(sensChan); err != nil {
			return err
		}

	}

	return nil
}
