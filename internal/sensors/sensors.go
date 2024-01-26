package sensors

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maxb-odessa/slog"

	"github.com/maxb-odessa/nonsens/internal/config"
)

func Start(conf *config.Config, sensChan chan *config.Sensor) error {

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

	// TBD: more source types, i.e. exec
	if sens.Type != "file" {
		return fmt.Errorf("sensor '%s' is of unsupported type (%s)", sens.Name, sens.Type)
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

	// TODO: keep open, rewind
	var err error
	reader := func() {

		if data, err := os.ReadFile(sens.Source); err == nil {
			s := strings.TrimSpace(string(data))
			if value, err := strconv.ParseFloat(s, 64); err == nil {
				sens.Priv.Lock()
				sens.Priv.CurrValue = math.Round(value / sens.Divider)
				sens.Priv.Online = true
				sens.Priv.Unlock()
			}
		}

		if err != nil {
			sens.Priv.Lock()
			sens.Priv.Online = false
			sens.Priv.Unlock()
		}

		sensChan <- sens
	}

	// start sensor poller
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
