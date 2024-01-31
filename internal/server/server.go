package server

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"sync"
	"time"

	mux "github.com/gorilla/mux"
	ws "github.com/gorilla/websocket"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/tmpl"
	"github.com/maxb-odessa/slog"
)

var toClientCh chan []byte
var wsChans map[string]chan []byte
var wsChansLock sync.Mutex
var templates tmpl.Tmpls

// uniq body id
const BODYID = "main"

var bodyData string

func Start(conf *config.Config, sensChan chan *config.Sensor) error {
	var err error

	mime.AddExtensionType(".css", "text/css")
	toClientCh = make(chan []byte, 8)
	wsChans = make(map[string]chan []byte, 0)

	go chanDispatcher(toClientCh)

	templates, err = tmpl.Load(conf.Server.Resources + "/templates")
	if err != nil {
		return err
	}

	if err = makeBody(conf); err != nil {
		return err
	}

	// start sending sysinfo
	//go sendSysinfo(templates)

	// start sensors events listening and processing
	go processSensors(templates, sensChan)

	// fire up the server
	go server(conf)

	return nil
}

func makeBody(conf *config.Config) error {
	var err error

	type Sen struct {
		Id   string
		Json string
	}

	type Grp struct {
		Id      string
		Sensors []*Sen
	}

	groups := make([][]*Grp, 0)

	// walk over columns
	for _, col := range conf.Sensors {

		// walk over sensors in this column and make a list of groups
		grList := make([]string, 0)
		for _, sens := range col {

			// skip disabled
			if sens.Disabled {
				continue
			}

			unique := true

			for _, g := range grList {
				// this group is already defined
				if sens.Group == g {
					unique = false
					break
				}
			}

			// record new group
			if unique {
				grList = append(grList, sens.Group)
			}

		}

		// populate group with corresponding sensors (preserve configured order)
		gr := make([]*Grp, 0)
		for _, g := range grList {
			ngr := new(Grp)
			ngr.Id = g
			ngr.Sensors = make([]*Sen, 0)
			for _, sens := range col {
				// skip disabled
				if sens.Disabled {
					continue
				}
				if g == sens.Group {
					js, _ := json.Marshal(sens)
					ngr.Sensors = append(ngr.Sensors, &Sen{Id: sens.Priv.Id, Json: string(js)})
				}
			}
			gr = append(gr, ngr)
		}

		groups = append(groups, gr)
	}

	// prepare the body with all groups and sensors placed
	bodyData, err = tmpl.ApplyByName("body", templates, groups)
	if err != nil {
		return err
	}

	return nil
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
		sens.Priv.Lock()
		body, err := tmpl.ApplyByName("sensor", templates, sens)
		sens.Priv.Unlock()
		if err != nil {
			slog.Warn("json.Marshal failed: %s", err)
			continue
		}

		msg := &ToClientMsg{
			Target: sens.Priv.Id,
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

func server(conf *config.Config) {
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

		wsChan := make(chan []byte, 8)
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
					slog.Debug(9, "Got from remote: %+v", string(msg))
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
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}

	srv.ListenAndServe()
}

type FromClientMsg struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	Group     string  `json:"group"`
	Column    int     `json:"column"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Disabled  bool    `json:"disabled"`
	Divider   float64 `json:"divider"`
	Poll      float64 `json:"poll"`
	Units     string  `json:"units"`
	Type      string  `json:"type"`
	Fractions int     `json:"fractions"`
	Color0    string  `json:"color0"`
	Color100  string  `json:"color100"`
}

func processClientMsg(msg []byte) {
	var data FromClientMsg

	err := json.Unmarshal(msg, &data)
	if err != nil {
		slog.Err("failed to unmarshal json from client: %s", err)
		return
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
