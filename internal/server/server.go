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
	"github.com/maxb-odessa/slog"
)

var toClientCh chan []byte
var wsChans map[string]chan []byte
var wsChansLock sync.Mutex
var templates tmpl.Tmpls
var bodyData string
var conf *config.Config

// uniq body id
const BODYID = "main"

func Run(cf *config.Config) error {
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
	go sendSensorsData()

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

func sendSensorsData() {

	sensChan := sensors.Chan()

	for sens := range sensChan {

		// apply template on that sensor
		sens.Lock()
		body, err := tmpl.ApplyByName("sensor", templates, sens)
		sens.Unlock()
		if err != nil {
			slog.Warn("templating sensor failed: %s", err)
			continue
		}

		msg := &ToClientMsg{
			Target: sens.Id(),
			Data:   body,
		}
		data, _ := json.Marshal(msg)

		// send data to the client
		slog.Debug(5, "sending sensor to server: %+v", msg)
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
				mtype, mdata, err := conn.ReadMessage()
				if err != nil || mtype == ws.CloseMessage {
					return
				}
				// got a message from the remote
				if mtype == ws.TextMessage {
					slog.Debug(5, "Got from remote: %+v", string(mdata))
					processFeedback(mdata)
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

	sr := &http.Server{
		Handler: router,
		Addr:    listen,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	sr.ListenAndServe()
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
