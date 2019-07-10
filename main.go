package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"github.com/h2non/filetype"
	log "github.com/schollz/logger"
	"github.com/vincent-petithory/dataurl"
)

type server struct {
	publicURL string
	port      string

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
	var flagPort, flagPublicURL string
	flag.StringVar(&flagPort, "port", "8001", "port")
	flag.StringVar(&flagPublicURL, "url", "", "public url to use")
	flag.BoolVar(&debug, "debug", false, "debug mode")
	flag.Parse()

	if debug {
		log.SetLevel("debug")
	} else {
		log.SetLevel("info")
	}

	if flagPublicURL == "" {
		flagPublicURL = "localhost:" + flagPort
	}
	if !strings.HasPrefix(flagPublicURL, "http") {
		flagPublicURL = "http://" + flagPublicURL
	}

	s := &server{
		port:      flagPort,
		conn:      make(map[string][]*Connection),
		publicURL: flagPublicURL,
	}

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
	log.Debugf("URL: %s, Referer: %s", r.URL.Path, r.Referer())
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
			PublicURL string
			Title     string
			HTML      string
		}
		return t.Execute(w, view{PublicURL: s.publicURL})
	} else {
		log.Debugf("attempting to find %s", r.URL.Path)

		pathToFile := r.URL.Path[1:]
		domain := strings.Split(r.URL.Path[1:], "/")[0]
		// check to make sure it has domain prepended
		piecesOfReferer := strings.Split(r.Referer(), "/")
		if len(piecesOfReferer) > 4 && strings.HasPrefix(r.Referer(),s.publicURL) {
			domain = piecesOfReferer[3]
		}

		// prefix the domain if it doesn't exist
		if !strings.HasPrefix(pathToFile, domain) {
			pathToFile = domain + "/" + pathToFile
			http.Redirect(w, r, "/"+pathToFile, 302)
			return
		}

		// add index.html if it doesn't exist
		if filepath.Ext(pathToFile) == "" {
			if string(pathToFile[len(pathToFile)-1]) != "/" {
				pathToFile += "/"
			}
			pathToFile += "index.html"
			http.Redirect(w, r, "/"+pathToFile, 302)
			return
		}

		var ipAddress string
		ipAddress, err = GetClientIPHelper(r)
		if err != nil {
			log.Debugf("could not determine ip: %s", err.Error())
		}

		var data string
		data, err = s.get(pathToFile, ipAddress)
		if err != nil {
			return
		}
		var dataURL *dataurl.DataURL
		dataURL, err = dataurl.DecodeString(data)
		if err != nil {
			return
		}
		contentType := dataURL.MediaType.ContentType()
		if contentType == "application/octet-stream" || contentType == "" {
			pathToFileExt := filepath.Ext(pathToFile)
			mimeType := filetype.GetType(pathToFileExt)
			contentType = mimeType.MIME.Value
			if contentType == "" {
				switch pathToFileExt {
				case ".css":
					contentType = "text/css"
				case ".js":
					contentType = "text/javascript"
				case ".html":
					contentType = "text/html"
				}
			}
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
	Type      string `json:"type"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	IPAddress string `json:"ip"`
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

func (s *server) get(filePath, ipAddress string) (payload string, err error) {
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
				Type:      "get",
				Message:   filePath,
				IPAddress: ipAddress,
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
		if p.Type == "get" {
			payload = p.Message
			if !p.Success {
				err = fmt.Errorf(payload)
			}
			return
		}
		if len(p.Message) > 10 {
			p.Message = p.Message[:10] + "..."
		}
		log.Debugf("recv: %+v", p)
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
