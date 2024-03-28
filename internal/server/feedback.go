package server

import (
	"encoding/json"
	"fmt"

	"nonsens/internal/config"
	"nonsens/internal/sensors"
	"nonsens/internal/sensors/sensor"
	"nonsens/internal/utils"

	"github.com/maxb-odessa/slog"
)

type SensorData struct {
	ToTop   bool           `json:"totop"`
	GroupId string         `json:"groupid"`
	Sensor  *sensor.Sensor `json:"sensor"`
}

type GroupData struct {
	Name   string `json:"name"`
	Column int    `json:"column"`
	ToTop  bool   `json:"totop"`
}

type FeedbackMsg struct {
	Action string      `json:"action"` // what to do: appply, save, scan, etc.
	Id     string      `json:"id"`     // taget id, group or sensor
	Sensor *SensorData `json:"sensor"`
	Group  *GroupData  `json:"group"`
}

func processFeedback(data []byte) {
	var msg FeedbackMsg
	needRefresh := false

	err := json.Unmarshal(data, &msg)
	if err != nil {
		slog.Err("Failed to unmarshal feedback data: %s", err)
		return
	}
	slog.Debug(9, "GOT MSG: %+v", msg)

	if msg.Sensor != nil {
		// modify sensor
		needRefresh = modifySensor(msg.Id, msg.Action, msg.Sensor)
	} else if msg.Group != nil {
		// modify group
		needRefresh = modifyGroup(msg.Id, msg.Action, msg.Group)
	} else {
		// settings command, not related to group or sensor
		switch msg.Action {
		// save config
		case "save":
			if err := conf.Save(); err != nil {
				errMsg := fmt.Sprintf("Config file save failed: %s", err)
				slog.Err(errMsg)
				sendInfo(errMsg)
			} else {
				confBackup = conf
				sendInfo("Configuration applied and saved")
			}
		// scan for sensors
		case "scan":
			sendInfo("Scanning for sernsors...")
			newConf := sensors.ScanAllSensors()
			if newConf != nil {
				sensors.StopAllSensors(conf)
				newConf.ImportServerData(conf)
				conf = newConf
				sensors.StartAllSensors(conf)
				needRefresh = true
				sendInfo("Scan complete!")
			} else {
				sendInfo("Scan failed...")
			}
		case "restore":
			if conf != confBackup {
				conf = confBackup
				needRefresh = true
				sendInfo("Configuration restored!")
			} else {
				sendInfo("No recent changes")
			}
		default:
			slog.Err("Undefined feedback action '%s'", msg.Action)
			return
		}
	}

	// rebuild the body and refresh it
	if needRefresh {
		// delete all empty columns, etc
		conf.Sanitize()
		makeMainPage()
		sendMainPage(toClientCh)
	}

}

func modifySensor(id string, action string, sData *SensorData) bool {
	var needReconfig bool

	// add new sensor
	if action == "new" {
		sData.Sensor.Prepare()
		_, _, gr := conf.FindGroupById(sData.GroupId)
		conf.AddSensor(sData.Sensor, gr)
		sensors.SetupSensor(sData.Sensor)
		sData.Sensor.Start(sensors.Chan())
		return true
	}

	// assume the sensor is always modified

	gr, se := conf.FindSensorById(id)
	if se == nil {
		slog.Warn("Sensor id '%s' not found", id)
		return false
	}

	if action == "remove" {
		se.Stop()
		conf.RemoveSensor(se)
		slog.Info("Removed sensor '%s'", se.Name)
		return true
	}

	se.Lock()
	defer se.Unlock()

	// device or input file changed - reconfig sensors
	if se.Options.Device != sData.Sensor.Options.Device || se.Options.Input != sData.Sensor.Options.Input {
		needReconfig = true
	}

	se.Widget.Name = utils.SafeHTML(sData.Sensor.Widget.Name)
	//se.Name = sData.Sensor.Name

	se.Options = sData.Sensor.Options
	se.Widget = sData.Sensor.Widget

	// group changed
	if gr.Id() != sData.GroupId {
		conf.MoveSensorToGroup(se, gr, sData.GroupId)
	}

	// move sensor to group top
	if sData.ToTop {
		conf.MoveSensorToGroupTop(se)
	}

	se.Stop()

	if needReconfig {
		sensors.SetupSensor(se)
	}

	se.Start(sensors.Chan())

	return true
}

func modifyGroup(id string, action string, gData *GroupData) bool {
	modified := false

	// add new sensor
	if action == "new" {
		gr := new(config.Group)
		gr.SetName(gData.Name)
		gr.SetId(utils.MakeUID())
		conf.AddGroup(gData.Column, gr)
		return true
	}

	ci, _, gr := conf.FindGroupById(id)
	if gr == nil {
		slog.Warn("Group id '%s' not found", id)
		return false
	}

	if action == "remove" {
		if len(gr.Sensors) == 0 {
			conf.RemoveGroup(gr)
			slog.Info("Removed empty group '%s'", gr.Name)
			return true
		}
		return false
	}

	// name changed
	if gr.Name != gData.Name {
		gr.Name = utils.SafeHTML(gData.Name)
		modified = true
	}

	// column changed
	if ci != gData.Column {

		// remove from old column
		conf.RemoveGroup(gr)

		// create new column if missed
		for gData.Column > len(conf.Columns)-1 {
			conf.AddColumn()
			// columns must be monotonic, don't allow change column from 1 to 7, but from 3 to 4 is ok
			gData.Column = len(conf.Columns) - 1
		}

		// add to new column
		conf.AddGroup(gData.Column, gr)

		modified = true
	}

	// move this group to column top
	if gData.ToTop {
		if conf.MoveGroupToTop(id) {
			modified = true
		}
	}

	return modified
}
