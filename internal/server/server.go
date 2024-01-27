package server

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"

	ws "github.com/gorilla/websocket"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/tmpl"
	"github.com/maxb-odessa/slog"
)

var toClientCh chan []byte

func init() {
	mime.AddExtensionType(".css", "text/css")
	toClientCh = make(chan []byte, 8)
}

var indexPage string

func Start(conf *config.Config, sensChan chan *config.Sensor) error {

	// prepare index page

	templates, err := tmpl.Load(conf.Server.Resources + "/templates")
	if err != nil {
		return err
	}

	type Sen struct {
		Id string
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

			// group name can not be empty
			if sens.Group == "" {
				sens.Group = sens.Name
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
					ngr.Sensors = append(ngr.Sensors, &Sen{Id: sens.Priv.Id})
				}
			}
			gr = append(gr, ngr)
		}

		groups = append(groups, gr)
	}

	// prepare the index page with all groups and sensors placed
	indexPage, err = tmpl.ApplyByName("index", templates, groups)
	if err != nil {
		return err
	}

	// start sensors events listening and processing
	go processSensors(templates, sensChan)

	// fire up the server
	go server(conf)

	return nil
}

func processSensors(templates tmpl.Tmpls, sensChan chan *config.Sensor) {
	type Msg struct {
		Id   string `json:"id"`
		Body string `json:"body"`
	}

	for sens := range sensChan {

		// apply template on that sensor
		sens.Priv.Lock()
		body, err := tmpl.ApplyByName("sensor", templates, sens)
		sens.Priv.Unlock()
		if err != nil {
			slog.Warn("json.Marshal failed: %s", err)
			continue
		}

		msg := &Msg{
			Id:   sens.Priv.Id,
			Body: body,
		}
		data, _ := json.Marshal(msg)

		// send data to the client
		slog.Debug(9, "sendig to server: %+v", data)
		select {
		case toClientCh <- data:
		default:
			slog.Debug(5, "http server queue is full, discarding sensor data")
		}
	}

}

func server(conf *config.Config) {
	mux := http.NewServeMux()

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

		defer func() {
			slog.Info("Websocket connection closed: %s", conn.RemoteAddr())
			conn.Close()
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
					slog.Debug(1, "Got from remote: %+v", string(msg))
				}
			}
		}

		go reader()

		for {

			select {
			case msg, ok := <-toClientCh:
				if !ok {
					return
				}
				slog.Debug(9, "will send to ws: %s", msg)
				if err = conn.WriteMessage(ws.TextMessage, msg); err != nil {
					slog.Err("Websocket send() failed: %s", err)
					return
				} else {
					slog.Debug(5, "ws sent: %q", string(msg))
				}
			}
		}
	}
	mux.HandleFunc("/ws", wsHandler)

	pageDir := os.ExpandEnv(conf.Server.Resources + "/webpage")
	slog.Debug(9, "Serving HTTP dir: %s", pageDir)
	// NB: that odd "nosniff" thingie
	mux.Handle("/img/", http.StripPrefix("/img/", http.FileServer(http.Dir(pageDir+"/img"))))
	mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir(pageDir+"/css"))))

	getIndex := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, indexPage)
	}
	mux.HandleFunc("/", getIndex)

	listen := conf.Server.Listen
	if listen == "" {
		listen = ":12345"
	}
	slog.Info("Listening at %s", listen)
	http.ListenAndServe(listen, mux)
}
