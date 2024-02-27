package server

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	ws "github.com/gorilla/websocket"

	"github.com/rafacas/sysstats"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors"
	"github.com/maxb-odessa/nonsens/internal/tmpl"
	"github.com/maxb-odessa/slog"
)

var (
	toClientCh   chan []byte
	wsChans      map[string]chan []byte
	wsChansLock  sync.Mutex
	templates    tmpl.Tmpls
	mainPageData string
	conf         *config.Config
	confBackup   *config.Config
)

func Run(cf *config.Config) error {
	var err error

	conf = cf
	confBackup = conf

	mime.AddExtensionType(".css", "text/css")
	toClientCh = make(chan []byte, 32)
	wsChans = make(map[string]chan []byte, 0)

	go chanDispatcher(toClientCh)

	templates, err = tmpl.Load(conf.Server.Resources + "/templates")
	if err != nil {
		return err
	}

	if err = makeMainPage(); err != nil {
		return err
	}

	// start sending sysinfo
	go sendSysinfo()

	// start sensors events listening and processing
	go sendSensorsData()

	// fire up the server
	go server()

	return nil
}

// prepare the main page with all groups and sensors placed
func makeMainPage() error {
	var err error

	type PageData struct {
		HostName string
		Config   *config.Config
	}

	hostName, err := os.Hostname()
	if err != nil {
		if hostName = os.Getenv("HOSTNAME"); hostName == "" {
			hostName = "(unknown)"
		}
	}

	data := PageData{
		HostName: strings.ToUpper(hostName),
		Config:   conf,
	}

	mainPageData, err = tmpl.ApplyByName("main", templates, data)

	return err
}

type ToClientMsg struct {
	Target string `json:"target"`
	Data   string `json:"data"`
}

func sendInfo(text string) {
	msg := &ToClientMsg{
		Target: "info",
		Data:   text,
	}

	data, _ := json.Marshal(msg)

	slog.Debug(9, "sending info to server: %+v", msg)

	select {
	case toClientCh <- data:
	default:
		slog.Warn("Server chan is full, discarding info message")
	}
}

func GetHostName() string {
	return "HostName Here"
}

func sendMainPage(ch chan []byte) {

	msg := &ToClientMsg{
		Target: "main",
		Data:   mainPageData,
	}

	data, _ := json.Marshal(msg)

	slog.Debug(9, "sending main page to server: %+v", msg)

	// can't skip this message - it's a main page
	ch <- data
}

func sendSysinfo() {
	var msg *ToClientMsg
	var data []byte

	if conf.SysinfoPoll <= 0 {
		conf.SysinfoPoll = 10
		slog.Warn("Invalid 'sysinfo poll' value, using 10 seconds")
	}

	sinfo := func() {

		// send current time
		msg = &ToClientMsg{
			Target: "sysinfo-time",
			Data:   time.Now().Format("Date: 2006-01-02 15:04:05"),
		}
		data, _ = json.Marshal(msg)

		select {
		case toClientCh <- data:
		default:
		}

		// send load averages
		la, _ := sysstats.GetLoadAvg()
		msg = &ToClientMsg{
			Target: "sysinfo-la",
			Data:   fmt.Sprintf("LA: %.2f, %.2f, %.2f", la.Avg1, la.Avg5, la.Avg15),
		}
		data, _ = json.Marshal(msg)

		select {
		case toClientCh <- data:
		default:
		}

		// send mem stats
		mem, _ := sysstats.GetMemStats()
		msg = &ToClientMsg{
			Target: "sysinfo-mem",
			Data:   fmt.Sprintf("Free: %d of %d MBytes", (mem["memused"]-mem["cached"]-mem["buffers"])/1024, mem["memtotal"]/1024),
		}
		data, _ = json.Marshal(msg)

		select {
		case toClientCh <- data:
		default:
		}

	}

	ticker := time.NewTicker(time.Duration(conf.SysinfoPoll) * time.Second)
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
			slog.Warn("Templating sensor failed: %s", err)
			continue
		}

		msg := &ToClientMsg{
			Target: sens.Id(),
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
				mtype, mdata, err := conn.ReadMessage()

				if err != nil {
					slog.Err("Websocket error: %s", err)
					return
				}

				switch mtype {
				case ws.CloseMessage:
					return
				case ws.TextMessage:
					slog.Debug(5, "Got from remote: %+v", string(mdata))
					processFeedback(mdata)
				}
			}
		}

		go reader()

		go sendMainPage(wsChan) // this blocks if chan is full

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
	slog.Info("Serving HTTP dir: %s", pageDir)

	// NB: that odd "nosniff" thingie
	router.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir(pageDir))))

	listen := conf.Server.Listen
	if listen == "" {
		listen = ":12345"
	}
	slog.Info("Listening at %s", listen)

	sr := &http.Server{
		Handler:      router,
		Addr:         listen,
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
