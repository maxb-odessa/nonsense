package server

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	ws "github.com/gorilla/websocket"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors"
	"github.com/maxb-odessa/nonsens/internal/tmpl"
	"github.com/maxb-odessa/nonsens/internal/utils"
	"github.com/maxb-odessa/slog"
)

var toClientCh chan []byte
var wsChans map[string]chan []byte
var wsChansLock sync.Mutex
var templates tmpl.Tmpls
var conf *config.Config
var bodyData string

// uniq body id
const BODYID = "main"

func Start(cf *config.Config, sensChan chan *config.Sensor) error {
	var err error

	conf = cf

	mime.AddExtensionType(".css", "text/css")
	toClientCh = make(chan []byte, 32)
	wsChans = make(map[string]chan []byte, 0)

	go chanDispatcher(toClientCh)

	templates, err = tmpl.Load(conf.Server.Resources + "/templates")
	if err != nil {
		return err
	}

	if err = makeBody(); err != nil {
		return err
	}

	// start sending sysinfo TODO
	//go sendSysinfo(templates)

	// start sensors events listening and processing
	go processSensors(templates, sensChan)

	// fire up the server
	go server()

	return nil
}

// prepare the body with all groups and sensors placed
func makeBody() error {
	var err error
	bodyData, err = tmpl.ApplyByName("body", templates, conf)
	return err
}

type ToClientMsg struct {
	Target string `json:"target"`
	Data   string `json:"data"`
}

func sendBody() {

	msg := &ToClientMsg{
		Target: BODYID,
		Data:   bodyData,
	}

	data, _ := json.Marshal(msg)

	slog.Debug(1, "sending body to server: %+v", msg)

	// can't skip this message - it's a body!
	toClientCh <- data
}

// TODO
func sendSysinfo(templates tmpl.Tmpls) {
	ticker := time.NewTicker(1 * time.Second)

	sinfo := func() {

	}

	for {
		select {
		case <-ticker.C:
			sinfo()
		}
	}

}

func processSensors(templates tmpl.Tmpls, sensChan chan *config.Sensor) {

	for sens := range sensChan {

		// apply template on that sensor
		sens.Pvt.Lock()
		body, err := tmpl.ApplyByName("sensor", templates, sens)
		sens.Pvt.Unlock()
		if err != nil {
			slog.Warn("templating sensor failed: %s", err)
			continue
		}

		msg := &ToClientMsg{
			Target: sens.Pvt.Id,
			Data:   body,
		}
		data, _ := json.Marshal(msg)

		// send data to the client
		slog.Debug(9, "sending sensor to server: %+v", msg)
		select {
		case toClientCh <- data:
		default:
			slog.Debug(5, "http server queue is full, discarding sensor data")
		}
	}

}

func server() {
	router := mux.NewRouter()

	wsHandler := func(w http.ResponseWriter, r *http.Request) {
		var upgrader = ws.Upgrader{
			ReadBufferSize:  8192,
			WriteBufferSize: 8192,
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Err("Websocket upgrade failed: %s", err)
			return
		}

		slog.Info("Websocket connected: %s", conn.RemoteAddr())

		wsChan := make(chan []byte, 16)
		registerChan(wsChan, conn.RemoteAddr().String())

		defer func() {
			slog.Info("Websocket connection closed: %s", conn.RemoteAddr())
			conn.Close()
			unregisterChan(conn.RemoteAddr().String())
			close(wsChan)
		}()

		reader := func() {
			for {
				// catch remote connection close
				mtype, msg, err := conn.ReadMessage()
				if err != nil || mtype == ws.CloseMessage {
					return
				}
				// got a message from the remote
				if mtype == ws.TextMessage {
					slog.Debug(5, "Got from remote: %+v", string(msg))
					processClientMsg(msg)
				}
			}
		}

		go reader()

		go sendBody() // this blocks if chan is full

		for {
			select {
			case msg, ok := <-wsChan:
				if !ok {
					return
				}
				slog.Debug(9, "will send to ws: %s", msg)
				if err = conn.WriteMessage(ws.TextMessage, msg); err != nil {
					slog.Err("Websocket send() failed: %s", err)
					return
				} else {
					slog.Debug(9, "ws sent: %q", string(msg))
				}
			}
		}
	}
	router.HandleFunc("/ws", wsHandler)

	pageDir := os.ExpandEnv(conf.Server.Resources + "/webpage")
	slog.Debug(9, "Serving HTTP dir: %s", pageDir)
	// NB: that odd "nosniff" thingie
	router.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir(pageDir))))
	/*
		router.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir(pageDir+"/"))))
		router.Handle("/img/", http.StripPrefix("/img/", http.FileServer(http.Dir(pageDir+"/img"))))
		router.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir(pageDir+"/css"))))
	*/

	listen := conf.Server.Listen
	if listen == "" {
		listen = ":12345"
	}
	slog.Info("Listening at %s", listen)

	srv := &http.Server{
		Handler: router,
		Addr:    listen,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	srv.ListenAndServe()
}

type Group struct {
	Name   string `json:"name"`
	Column int    `json:"column"`
	Top    bool   `json:"top"`
}

type FromClientMsg struct {
	Action string         `json:"action"`
	Id     string         `json:"id"`
	Sensor *config.Sensor `json:"sensor"`
	Group  *Group         `json:"group"`
}

func processClientMsg(msg []byte) {
	var data FromClientMsg
	needRefresh := false

	err := json.Unmarshal(msg, &data)
	if err != nil {
		slog.Err("failed to unmarshal json from client: %s", err)
		return
	}
	slog.Info("GOT: %+v", data)

	// modify sensor
	if data.Sensor != nil {
		needRefresh = true

		_, _, se := conf.FindSensorById(data.Id)
		if se == nil {
			slog.Warn("sensor '%s' not found", data.Id)
			return
		}
		slog.Info("se: %+v", *data.Sensor)

		se.Pvt.Lock()

		// this chnage requires sensor restart
		restart := false
		if se.Options.Poll != data.Sensor.Options.Poll {
			restart = true
		}

		se.Name = data.Sensor.Name
		se.Disabled = data.Sensor.Disabled
		se.Options = data.Sensor.Options
		se.Widget = data.Sensor.Widget

		if restart {
			sensors.StopSensor(se)
			sensors.StartSensor(se)
			// TBD: move config.Sensor to sensors, write methods
		}

		se.Pvt.Unlock()
		// TBD group placing

	}

	// modify this group
	if data.Group != nil {

		colIdx, grIdx, gr := conf.FindGroupById(data.Id)
		if gr == nil {
			slog.Warn("group '%s' not found", data.Id)
			return
		}

		// name changed
		if data.Group.Name != data.Id {
			needRefresh = true
			gr.Name = utils.SafeHTML(data.Group.Name)
		}

		// column changed
		if colIdx != data.Group.Column {
			needRefresh = true

			// remove from old column
			conf.Columns[colIdx].Groups = append(conf.Columns[colIdx].Groups[:grIdx], conf.Columns[colIdx].Groups[grIdx+1:]...)

			// create new column if missed
			for data.Group.Column > len(conf.Columns)-1 {
				conf.Columns = append(conf.Columns, new(config.Column))
				// columns must be monotonic, don't allow change column from 1 to 7, but from 3 to 4 is ok
				data.Group.Column = len(conf.Columns) - 1
			}

			// add to new column
			if len(conf.Columns[data.Group.Column].Groups) < 1 {
				conf.Columns[data.Group.Column].Groups = make([]*config.Group, 0)
			}
			conf.Columns[data.Group.Column].Groups = append(conf.Columns[data.Group.Column].Groups, gr)

			// delete all empty columns
			for i := 0; i < len(conf.Columns); i++ {
				if len(conf.Columns[i].Groups) == 0 {
					conf.Columns = append(conf.Columns[:i], conf.Columns[i+1:]...)
				}
			}
		}

		// move this group to column top
		if data.Group.Top {
			needRefresh = true

			// get group position again in case of it was move between columns
			colIdx, grIdx, gr := conf.FindGroupById(data.Id)

			for i := grIdx; i > 0; i-- {
				conf.Columns[colIdx].Groups[i] = conf.Columns[colIdx].Groups[i-1]
			}
			conf.Columns[colIdx].Groups[0] = gr

		}
	}

	if data.Action == "save" {
		// save config file
	}

	// rebuild the body and refresh it
	if needRefresh {
		makeBody()
		sendBody()
	}

}

func registerChan(ch chan []byte, id string) {
	wsChansLock.Lock()
	wsChans[id] = ch
	wsChansLock.Unlock()
	slog.Debug(9, "REG chan id %s", id)
}

func unregisterChan(id string) {
	wsChansLock.Lock()
	if _, ok := wsChans[id]; ok {
		delete(wsChans, id)
		slog.Debug(9, "UNREG chan id %s", id)
	}
	wsChansLock.Unlock()
}

func chanDispatcher(ch chan []byte) {
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				continue
			}
			wsChansLock.Lock()
			for id, wsCh := range wsChans {
				select {
				case wsCh <- msg:
					slog.Debug(9, "SEND chan id %s", id)
				default:
					slog.Debug(9, "chan send to %s failed", id)
				}
			}
			wsChansLock.Unlock()
		}
	}
}
