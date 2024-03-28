package sensor

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"nonsens/internal/gradient"
	"nonsens/internal/utils"

	"github.com/maxb-odessa/slog"
)

// config data read from file
type Sensor struct {

	// privat data
	pvt struct {
		sync.Mutex
		active         bool
		done           chan bool // sensor is done
		cancelFunc     func()    // ctx cancelling func
		id             string    // uniq id
		input          string    // full path to sensor input file, may vary across reboots
		inputFd        *os.File  // opened input file descriptor
		inputBuf       []byte
		fractionsRatio float64 // calculated fractions ratio to be shown
		percentier     float64 // calculated (max - min ) * 100
	} `json:"-"`

	// runtime data, not for save
	Runtime struct {
		Dir          string              // full path to sensor dir (may change over boot so it's not persistent)
		Value        float64             // current read value
		Percents     float64             // calculated percents (based on Value and Min/Max)
		AntiPercents float64             // = (100 - percents) used for gauges
		Color        string              // widget gradient color at this percent value
		Gradient     *gradient.Gradient3 // prepared widget gradient data
	} `json:"-"`

	// configured data
	Name    string `json:"name"`    // full sensor name (for logging mostly)
	Offline bool   `json:"offline"` // is offline?

	Options struct {
		Device  string  `json:"device"`  // device id as in /sys/devices/..., i.e. 0000:09:00.0
		Input   string  `json:"input"`   // short input data file name relative to /sys/class/hwmon/hwmonX/
		Min     float64 `json:"min"`     // min value
		Max     float64 `json:"max"`     // max value
		Divider float64 `json:"divider"` // value divider, i.e. 1000 for temperature values like 42123 which 42.123 deg
		Poll    int     `json:"poll"`    // poll interval, in milliseconds
	} `json:"options"`

	Widget struct {
		Name         string  `json:"name"`      // visible sensor name
		Units        string  `json:"units"`     // suffix shown value with units string
		Fractions    int     `json:"fractions"` // show only this number of value fractions, i.e. 2 = 1.23 for 1.23456 valuea
		Color        string  `json:"color"`     // text color
		Color0       string  `json:"color0"`    // min value color (at 0%)
		ColorN       string  `json:"colorn"`    // min value color (at N%)
		Color100     string  `json:"color100"`  // max value color (at 100%)
		ColorNP      float64 `json:"colornp"`   // colorN percents position
		ShowGradient bool    `json:"gradient"`  // show gradient over gauge?
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

func (s *Sensor) Prepare() {
	s.pvt.id = utils.MakeUID()
	s.pvt.done = make(chan bool, 0)
}

func (s *Sensor) SetDefaults() {
	s.Options.Divider = 1.0
	s.Options.Poll = 1000
	s.Widget.Units = "units"
	s.Widget.Fractions = 1
	s.Widget.Color0 = "#00FF00"
	s.Widget.ColorN = "#FFFF00"
	s.Widget.Color100 = "#FF0000"
	s.Widget.ColorNP = 50.0
}

func (sens *Sensor) Start(sensChan chan *Sensor) error {

	// already running
	if sens.pvt.active {
		slog.Warn("Sensor '%s' already running", sens.Name)
		return nil
	}

	// some params checking
	if sens.Options.Divider == 0.0 {
		slog.Info("Forcing sensor '%s' divider to 1.0", sens.Name)
		sens.Options.Divider = 1.0
	}

	if sens.Options.Poll < 500 {
		slog.Info("Forcing sensor '%s' poll interval to 1 second", sens.Name)
		sens.Options.Poll = 1000
	}

	if sens.Widget.Fractions < 0 || sens.Widget.Fractions > 8 {
		slog.Info("Forcing sensor '%s' fractions to 0", sens.Name)
		sens.Widget.Fractions = 0
	} else {
		sens.pvt.fractionsRatio = math.Pow(10, float64(sens.Widget.Fractions))
	}

	if sens.Options.Min >= sens.Options.Max {
		sens.Options.Max = sens.Options.Min + 1
		slog.Info("Forcing sensor '%s' min/max to %f/%f", sens.Name, sens.Options.Min, sens.Options.Max)
	}

	if !sens.Widget.ShowGradient {
		sens.Runtime.Gradient = new(gradient.Gradient3)
		sens.Runtime.Gradient.Make(sens.Widget.Color0, sens.Widget.ColorN, sens.Widget.Color100, sens.Widget.ColorNP)
	}

	sens.Name = sens.Options.Device + "/" + sens.Options.Input

	sens.pvt.percentier = (sens.Options.Max - sens.Options.Min) / 100.0

	sens.pvt.inputBuf = make([]byte, 64)

	updater := func() {

		sens.Lock()

		if value, err := sens.readInput(); err != nil {
			sens.Offline = true
		} else {

			sens.Offline = false

			// apply divider if defined
			if sens.Options.Divider != 1.0 {
				sens.Runtime.Value = value / sens.Options.Divider
			} else {
				sens.Runtime.Value = value
			}

			// round to fractions if defined
			if sens.Widget.Fractions > 0 {
				sens.Runtime.Value = math.Round(sens.Runtime.Value*sens.pvt.fractionsRatio) / sens.pvt.fractionsRatio
			} else {
				sens.Runtime.Value = math.Round(sens.Runtime.Value)
			}

			// auto-adjust min/max values
			if sens.Runtime.Value > sens.Options.Max {
				slog.Warn("Max value for sensor '%s' is too low: value=%f, max=%f), adjusting", sens.Name, sens.Runtime.Value, sens.Options.Max)
				sens.Options.Max = sens.Runtime.Value
				sens.pvt.percentier = (sens.Options.Max - sens.Options.Min) / 100.0
			}

			if sens.Runtime.Value < sens.Options.Min {
				slog.Warn("Min value for sensor '%s' is too high: value=%f, min=%f), adjusting", sens.Name, sens.Runtime.Value, sens.Options.Min)
				sens.Options.Min = sens.Runtime.Value
				sens.pvt.percentier = (sens.Options.Max - sens.Options.Min) / 100.0
			}

			// calc percents
			sens.Runtime.Percents = (sens.Runtime.Value - sens.Options.Min) / sens.pvt.percentier
			sens.Runtime.AntiPercents = 100.0 - sens.Runtime.Percents

			// calc current gradient color value if not showing whole gradient gauge
			if !sens.Widget.ShowGradient {
				sens.Runtime.Color = sens.Runtime.Gradient.ColorAt(sens.Runtime.Percents).String()
			}

			slog.Debug(5, "sensor '%s' value=%f percents=%f", sens.Name, sens.Runtime.Value, sens.Runtime.Percents)
		}

		sens.Unlock()

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
		ticker := time.NewTicker(interval * time.Millisecond)

		defer func() {
			slog.Info("Stopped sensor '%s'", sens.Name)
			ticker.Stop()
			sens.closeInput()
			sens.pvt.active = false
			sens.pvt.done <- true
		}()

		slog.Info("Started sensor '%s'", sens.Name)

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

	}()

	return nil
}

func (s *Sensor) Stop() {
	if s.pvt.active && s.pvt.cancelFunc != nil {
		s.pvt.cancelFunc()
		// wait for sensor to finish its job
		<-s.pvt.done
	}
}

func (s *Sensor) readInput() (val float64, err error) {

	defer func() {
		if err != nil {
			s.closeInput()
			s.pvt.active = false
		} else {
			s.pvt.active = true
		}
	}()

	if s.pvt.inputFd == nil {
		if s.pvt.inputFd, err = os.Open(s.pvt.input); err != nil {
			return
		}
	}

	if _, err = s.pvt.inputFd.Seek(0, io.SeekStart); err != nil {
		return
	}

	if num, e := s.pvt.inputFd.Read(s.pvt.inputBuf); e != nil {
		if e != io.EOF {
			err = e
			return
		}
	} else {
		val, err = strconv.ParseFloat(strings.TrimSpace(string(s.pvt.inputBuf[:num])), 64)
	}

	return
}

func (s *Sensor) closeInput() {
	if s.pvt.inputFd != nil {
		s.pvt.inputFd.Close()
		s.pvt.inputFd = nil
	}
}
