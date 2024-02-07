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

	// configure sensors via hwmon kernel subsystem
	if err := setupAllSensors(conf); err != nil {
		return err
	}

	sensChan = make(chan *sensor.Sensor, 64)

	// start sensors
	for _, sens := range conf.AllSensors() {
		sens.Start(sensChan)
	}

	return nil
}
