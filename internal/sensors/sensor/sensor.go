package sensor

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxb-odessa/nonsens/internal/utils"
	"github.com/maxb-odessa/slog"
)

// config data read from file
type Sensor struct {

	// privat data
	pvt struct {
		sync.Mutex
		active         bool
		cancelFunc     func()  // ctx cancelling func
		id             string  // uniq id
		input          string  // full path to sensor input file, may vary across reboots
		fractionsRatio float64 // calculated fractions ratio to be shown
	} `json:"-"`

	// runtime data, not for save
	Runtime struct {
		Value        float64 // current read value
		Percents     float64 // calculated percents (based on Min and Max)
		AntiPercents float64 // = (100 - percents) used for gauges
	} `json:"-"`

	// configured data
	Name    string `json:"name"`    // name to show
	Offline bool   `json:"offline"` // is offline?

	Options struct {
		Device  string  `json:"device"`  // device id as in /sys/devices/..., i.e. 0000:09:00.0
		Input   string  `json:"input"`   // short input data file name relative to /sys/class/hwmon/hwmonX/
		Min     float64 `json:"min"`     // min value
		Max     float64 `json:"max"`     // max value
		Divider float64 `json:"divider"` // value divider, i.e. 1000 for temperature values like 42123 which 42.123 deg
		Poll    float64 `json:"poll"`    // poll interval, in seconds
	} `json:"options"`

	Widget struct {
		Type      string `json:"type"`      // gauge, static, text, blink, etc (TBD)
		Units     string `json:"units"`     // suffix shown value with units string
		Fractions int    `json:"fractions"` // show only this number of value fractions, i.e. 2 = 1.23 for 1.23456 value
		Color     string `json:"color"`     // text color
		Color0    string `json:"color0"`    // min value color (at 0%)
		Color100  string `json:"color100"`  // max value color (at 100%)
	} `json:"widget"`
}

func (s *Sensor) Json() string {
	j, _ := json.Marshal(s)
	return string(j)
}

func (s *Sensor) Id() string {
	return s.pvt.id
}

func (s *Sensor) SetId(i string) {
	s.pvt.id = i
}

func (s *Sensor) Lock() {
	s.pvt.Lock()
}

func (s *Sensor) Unlock() {
	s.pvt.Unlock()
}

func (s *Sensor) SetInput(i string) {
	s.pvt.input = i
}

func (s *Sensor) Active() bool {
	return s.pvt.active
}

func (sens *Sensor) Start(sensChan chan *Sensor) error {
	var err error

	// already running
	if sens.pvt.active {
		slog.Warn("sensor '%s' already running", sens.Name)
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
		sens.pvt.fractionsRatio = math.Pow(10, float64(sens.Widget.Fractions))
	}

	if sens.Options.Min > sens.Options.Max {
		slog.Info("forcing sensor '%s' min/max to %f", sens.Name, sens.Options.Max)
		sens.Options.Min = sens.Options.Max
	}

	sens.Name = utils.SafeHTML(sens.Name)

	updater := func() {

		if data, err := os.ReadFile(sens.pvt.input); err == nil {

			s := strings.TrimSpace(string(data))

			if value, err := strconv.ParseFloat(s, 64); err == nil {

				sens.Lock()

				// this senseor is operational
				sens.Offline = false

				sens.Runtime.Value = value

				// apply divider if defined
				if sens.Options.Divider != 1.0 {
					sens.Runtime.Value = value / sens.Options.Divider
				}

				// round to fractions if defined
				if sens.Widget.Fractions > 0 {
					sens.Runtime.Value = math.Round(sens.Runtime.Value*sens.pvt.fractionsRatio) / sens.pvt.fractionsRatio
				} else {
					sens.Runtime.Value = math.Round(sens.Runtime.Value)
				}
				// calc percents if we can
				if sens.Options.Min != sens.Options.Max {
					sens.Runtime.Percents = sens.Runtime.Value / sens.Options.Max * 100.0
					if sens.Runtime.Percents > 100.0 {
						sens.Runtime.Percents = 100.0
						slog.Warn("Max value for sensor '%s' is too low (%f < %f)", sens.Name, sens.Options.Max, sens.Runtime.Value)
						// TODO auto adjust Max limit?
					}
					sens.Runtime.AntiPercents = 100.0 - sens.Runtime.Percents
				}

				sens.Unlock()

				slog.Debug(5, "sensor '%s' value=%f percents=%f", sens.Name, sens.Runtime.Value, sens.Runtime.Percents)
			}
		}

		if err != nil {
			sens.Lock()
			sens.Offline = true
			sens.Unlock()
		}

		select {
		case sensChan <- sens:
		default:
			slog.Debug(1, "sensors queue is full, discarding sensor data")
		}
	}

	// start sensor poller
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		sens.pvt.cancelFunc = cancel
		sens.pvt.active = true

		interval := time.Duration(sens.Options.Poll)
		ticker := time.NewTicker(interval * time.Second)

		slog.Info("Started sensor '%s/%s'", sens.Name, sens.Options.Device)

		updater() // to collect initial data

		loop := true
		for loop {
			select {
			case <-ctx.Done():
				loop = false
				break
			case <-ticker.C:
				updater()
			}
		}

		ticker.Stop()
		sens.pvt.active = false
		slog.Info("Stopped sensor '%s/%s'", sens.Name, sens.Options.Device)

	}()

	return nil
}

func (s *Sensor) Stop() {
	if s.pvt.active && s.pvt.cancelFunc != nil {
		s.pvt.cancelFunc()
	}
}
