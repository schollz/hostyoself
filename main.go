package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/schollz/logger"
	"github.com/vincent-petithory/dataurl"
	"github.com/h2non/filetype"
)

type server struct {
	port string

	// connections stored as map of domain -> connections
	conn map[string][]*Connection
	sync.Mutex
}

// Connection determine what can be held
type Connection struct {
	ID      int
	Joined  time.Time
	Domain  string
	LastGet string
	ws      *WebsocketConn
}

type WebsocketConn struct {
	ws *websocket.Conn
	sync.Mutex
}

func NewWebsocket(ws *websocket.Conn) *WebsocketConn {
	return &WebsocketConn{
		ws: ws,
	}
}

func (ws *WebsocketConn) Send(p Payload) (err error) {
	ws.Lock()
	defer ws.Unlock()
	err = ws.ws.WriteJSON(p)
	return
}

func (ws *WebsocketConn) Receive() (p Payload, err error) {
	ws.Lock()
	defer ws.Unlock()
	err = ws.ws.ReadJSON(&p)
	return
}

func main() {
	var debug bool
	var flagPort string
	flag.StringVar(&flagPort, "port", "8001", "port")
	flag.BoolVar(&debug, "debug", false, "debug mode")
	flag.Parse()

	if debug {
		log.SetLevel("debug")
	} else {
		log.SetLevel("info")
	}

	s := new(server)
	s.Lock()
	s.port = flagPort
	s.conn = make(map[string][]*Connection)
	s.Unlock()
	s.serve()
}

func (s *server) serve() (err error) {
	log.Infof("listening on :%s", s.port)
	http.HandleFunc("/", s.handler)
	return http.ListenAndServe(fmt.Sprintf(":%s", s.port), nil)
}

func (s *server) handler(w http.ResponseWriter, r *http.Request) {
	t := time.Now().UTC()
	err := s.handle(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(err)
	}
	log.Infof("%v %v %v %s\n", r.RemoteAddr, r.Method, r.URL.Path, time.Since(t))
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) (err error) {

	// very special paths
	if r.URL.Path == "/robots.txt" {
		// special path
		w.Write([]byte(`User-agent: * 
Disallow: /`))
	} else if r.URL.Path == "/ws" {
		return s.handleWebsocket(w, r)
	} else if r.URL.Path == "/favicon.ico" {
		err = fmt.Errorf("not implemented")
		return
	} else if r.URL.Path == "/" {
		var t *template.Template
		b, _ := ioutil.ReadFile("templates/view.html")
		t, err = template.New("view").Parse(string(b))
		if err != nil {
			log.Error(err)
			return err
		}
		type view struct {
			Title string
			HTML  string
		}
		return t.Execute(w, view{})
	} else {
		log.Debugf("attempting to find %s", r.URL.Path)
		var data string
		data, err = s.get(r.URL.Path[1:])
		if err != nil {
			return
		}
		var dataURL *dataurl.DataURL
		dataURL, err = dataurl.DecodeString(data)
		if err != nil {
			return
		}
		contentType := dataURL.MediaType.ContentType()
		if contentType == "application/octet-stream" {
			mimeType := filetype.GetType(r.URL.Path[1:])
			contentType = mimeType.MIME.Value
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(dataURL.Data)
		return
	}
	return
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Payload struct {
	// message meta
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p Payload) String() string {
	b, _ := json.Marshal(p)
	return string(b)
}

func (s *server) handleWebsocket(w http.ResponseWriter, r *http.Request) (err error) {
	// handle websockets on this page
	c, errUpgrade := wsupgrader.Upgrade(w, r, nil)
	if errUpgrade != nil {
		return errUpgrade
	}

	var conn *Connection

	log.Debugf("%s connected", c.RemoteAddr().String())

	var p Payload
	err = c.ReadJSON(&p)
	if err != nil {
		log.Debug(err)
		c.Close()
		return
	}
	log.Debugf("recv: %s", p)

	if !(p.Type == "domain" && p.Message != "") {
		err = fmt.Errorf("got wrong type/domain: %s/%s", p.Type, p.Message)
		log.Debug(err)
		c.Close()
		return
	}

	domain := strings.TrimSpace(p.Message)

	// create domain if it doesn't exist
	s.Lock()
	if _, ok := s.conn[domain]; !ok {
		s.conn[domain] = []*Connection{}
	}
	// register the new connection in the domain
	conn = &Connection{
		ID:     len(s.conn[domain]),
		Domain: domain,
		Joined: time.Now(),
		ws:     NewWebsocket(c),
	}
	s.conn[domain] = append(s.conn[domain], conn)
	log.Debugf("added: %+v", conn)
	s.Unlock()

	err = conn.ws.Send(Payload{
		Type:    "message",
		Message: "connected",
		Success: true,
	})

	return
}

func (s *server) get(filePath string) (payload string, err error) {
	log.Debugf("requesting %s", filePath)
	domain := strings.Split(filePath, "/")[0]

	var connections []*Connection
	s.Lock()
	if _, ok := s.conn[domain]; ok {
		connections = s.conn[domain]
	}
	s.Unlock()
	if connections == nil {
		err = fmt.Errorf("no connections available for domain %s", domain)
		return
	}

	// loop through connections and try to get one to serve the file
	for _, conn := range connections {
		var p Payload
		p, err = func() (p Payload, err error) {
			err = conn.ws.Send(Payload{
				Type:    "get",
				Message: filePath,
			})
			if err != nil {
				return
			}
			p, err = conn.ws.Receive()
			return
		}()
		if err != nil {
			log.Debug(err)
			s.DumpConnection(domain, conn.ID)
			continue
		}
		log.Debugf("recv: %+v", p)
		if p.Type == "get" {
			payload = p.Message
			if !p.Success {
				err = fmt.Errorf(payload)
			}
			return
		}
		err = fmt.Errorf("invalid response")
		break
	}
	return
}

func (s *server) DumpConnection(domain string, id int) (err error) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.conn[domain]; !ok {
		err = fmt.Errorf("domain %s not found", domain)
		log.Debug(err)
		return
	}
	for i, conn := range s.conn[domain] {
		if conn.ID == id {
			log.Debugf("dumping connection %s/%d", domain, id)
			s.conn[domain] = remove(s.conn[domain], i)
			return
		}
	}
	err = fmt.Errorf("could not find %s/%d to dump", domain, id)
	return
}

func remove(slice []*Connection, s int) []*Connection {
	return append(slice[:s], slice[s+1:]...)
}
