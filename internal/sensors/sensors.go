package sensors

import (
	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors/sensor"
)

var sensChan chan *sensor.Sensor

func Chan() chan *sensor.Sensor {
	return sensChan
}

func Run(conf *config.Config) error {

	sensChan = make(chan *sensor.Sensor, 64)

	// configure sensors via hwmon kernel subsystem
	if err := setupAllSensors(conf); err != nil {
		return err
	}

	StartAllSensors(conf)

	return nil
}

func StartAllSensors(conf *config.Config) {
	for _, sens := range conf.AllSensors() {
		sens.Start(sensChan)
	}
}

func StopAllSensors(conf *config.Config) {
	for _, sens := range conf.AllSensors() {
		sens.Stop()
	}
}
