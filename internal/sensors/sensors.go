package sensors

import (
	"time"

	"github.com/maxb-odessa/nonsens/internal/config"
)

func Start(conf *config.Config, sensChan chan *config.Sensor) error {

	go func() {
		for _, sens := range conf.Sensors {
			time.Sleep(1 * time.Second)
			for _, s := range sens {
				time.Sleep(1 * time.Second)
				sensChan <- s
			}
		}
	}()

	return nil
}
