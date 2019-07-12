package server

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/h2non/filetype"
	"github.com/schollz/hostyoself/pkg/namesgenerator"
	"github.com/schollz/hostyoself/pkg/utils"
	"github.com/schollz/hostyoself/pkg/wsconn"
	log "github.com/schollz/logger"
	"github.com/vincent-petithory/dataurl"
)

type server struct {
	publicURL string
	port      string

	// connections stored as map of domain -> connections
	conn map[string][]*connection
	sync.Mutex
}

// connection determine what can be held
type connection struct {
	ID      int
	Joined  time.Time
	Domain  string
	Key     string
	LastGet string
	ws      *wsconn.WebsocketConn
}

func New(publicURL, port string) *server {
	return &server{
		publicURL: publicURL,
		port:      port,
		conn:      make(map[string][]*connection),
	}
}

func (s *server) Run() (err error) {
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
Disallow:`))
	} else if r.URL.Path == "/ws" {
		return s.handleWebsocket(w, r)
	} else if r.URL.Path == "/favicon.ico" {
		err = fmt.Errorf("not implemented")
		return
	} else if strings.HasPrefix(r.URL.Path, "/static") {
		var b []byte
		b, err = Asset(r.URL.Path[1:])
		if err != nil {
			err = fmt.Errorf("resource '%s' not found", r.URL.Path[1:])
			return
		}
		var contentType string
		switch filepath.Ext(r.URL.Path) {
		case ".css":
			contentType = "text/css"
		case ".js":
			contentType = "text/javascript"
		case ".html":
			contentType = "text/html"
		case ".png":
			contentType = "image/png"
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(b)
		return
	} else if r.URL.Path == "/" {
		var t *template.Template
		b, _ := Asset("templates/view.html")
		t, err = template.New("view").Parse(string(b))
		if err != nil {
			log.Error(err)
			return err
		}
		type view struct {
			PublicURL       template.JS
			GeneratedDomain string
			GeneratedKey    string
		}
		return t.Execute(w, view{
			PublicURL:       template.JS(s.publicURL),
			GeneratedDomain: namesgenerator.GetRandomName(),
			GeneratedKey:    utils.RandStringBytesMaskImpr(6),
		})
	} else {
		// get IP address
		var ipAddress string
		ipAddress, err = utils.GetClientIPHelper(r)
		if err != nil {
			log.Debugf("could not determine ip: %s", err.Error())
		}

		log.Debugf("attempting to find %s", r.URL.Path)

		// determine file path and the domain
		pathToFile := r.URL.Path[1:]
		domain := strings.Split(r.URL.Path[1:], "/")[0]
		// clean domain
		domain = strings.Replace(strings.ToLower(strings.TrimSpace(domain)), " ", "-", -1)
		if !s.isdomain(domain) {
			log.Debugf("getting referer")
			// if there is a referer, try to obtain the domain from referer
			piecesOfReferer := strings.Split(r.Referer(), "/")
			if len(piecesOfReferer) > 4 && strings.HasPrefix(r.Referer(), s.publicURL) {
				domain = piecesOfReferer[3]
				domain = strings.Replace(strings.ToLower(strings.TrimSpace(domain)), " ", "-", -1)
			}
		}

		// prefix the domain if it doesn't exist
		if !strings.HasPrefix(pathToFile, domain) {
			pathToFile = domain + "/" + pathToFile
			if filepath.Ext(pathToFile) == "" {
				pathToFile += "/"
			}
			http.Redirect(w, r, "/"+pathToFile, 302)
			return
		}

		// add slash if doesn't exist
		if filepath.Ext(pathToFile) == "" && string(r.URL.Path[len(r.URL.Path)-1]) != "/" {
			http.Redirect(w, r, r.URL.Path+"/", 302)
			return
		}

		// trim prefix to get the path to file
		pathToFile = strings.TrimPrefix(pathToFile, domain)
		if len(pathToFile) == 0 || string(pathToFile[0]) == "/" {
			if len(pathToFile) <= 1 {
				pathToFile = "index.html"
			} else {
				pathToFile = pathToFile[1:]
			}
		}
		log.Debugf("pathToFile: %s", pathToFile)

		// send GET request to websockets
		var data string
		data, err = s.get(domain, pathToFile, ipAddress)
		if err != nil {
			// try index.html if it doesn't exist
			if filepath.Ext(pathToFile) == "" {
				if string(pathToFile[len(pathToFile)-1]) != "/" {
					pathToFile += "/"
				}
				pathToFile += "index.html"
				log.Debugf("trying 2nd try to get: %s", pathToFile)
				data, err = s.get(domain, pathToFile, ipAddress)
			}
			if err != nil {
				// try one more time
				if strings.HasSuffix(pathToFile, "/index.html") {
					pathToFile = strings.TrimSuffix(pathToFile, "/index.html")
					log.Debugf("trying 3rd try to get: %s", pathToFile)
					data, err = s.get(domain, pathToFile, ipAddress)
				}
				if err != nil {
					log.Debugf("problem getting: %s", err.Error())
					return
				}
			}
		}

		// decode the data URI
		var dataURL *dataurl.DataURL
		dataURL, err = dataurl.DecodeString(data)
		if err != nil {
			log.Errorf("problem decoding '%s': %s", data, err.Error())
			return
		}

		// determine the content type
		var contentType string
		switch filepath.Ext(pathToFile) {
		case ".css":
			contentType = "text/css"
		case ".js":
			contentType = "text/javascript"
		case ".html":
			contentType = "text/html"
		}
		if contentType == "" {
			contentType = dataURL.MediaType.ContentType()
			if contentType == "application/octet-stream" || contentType == "" {
				pathToFileExt := filepath.Ext(pathToFile)
				mimeType := filetype.GetType(pathToFileExt)
				contentType = mimeType.MIME.Value
			}
		}
		log.Debugf("%s/%s (%s)", domain, pathToFile, contentType)

		// write the data to the requester
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

func (s *server) handleWebsocket(w http.ResponseWriter, r *http.Request) (err error) {
	// handle websockets on this page
	c, errUpgrade := wsupgrader.Upgrade(w, r, nil)
	if errUpgrade != nil {
		log.Error(errUpgrade)
		return nil
	}
	ws := wsconn.New(c)

	log.Debugf("%s connected", c.RemoteAddr().String())

	p, errRead := ws.Receive()
	if errRead != nil {
		log.Debug(errRead)
		ws.Close()
		return
	}
	log.Debugf("recv: %s", p)

	if !(p.Type == "domain" && p.Message != "" && p.Key != "") {
		err = fmt.Errorf("got wrong type/domain: %s/%s", p.Type, p.Message)
		log.Debug(err)
		ws.Close()
		return nil
	}

	domain := strings.Replace(strings.ToLower(strings.TrimSpace(p.Message)), " ", "-", -1)

	// create domain if it doesn't exist
	s.Lock()
	if _, ok := s.conn[domain]; !ok {
		s.conn[domain] = []*connection{}
	}
	// register the new connection in the domain
	s.conn[domain] = append(s.conn[domain], &connection{
		ID:     len(s.conn[domain]),
		Domain: domain,
		Joined: time.Now(),
		Key:    p.Key,
		ws:     ws,
	})
	log.Debugf("added: %+v", s.conn)
	s.Unlock()

	err = ws.Send(wsconn.Payload{
		Type:    "domain",
		Message: domain,
		Success: true,
	})
	if err != nil {
		log.Error(err)
	}
	return nil
}

func (s *server) isdomain(domain string) bool {
	s.Lock()
	_, ok := s.conn[domain]
	s.Unlock()
	return ok
}

func (s *server) get(domain, filePath, ipAddress string) (payload string, err error) {
	var connections []*connection
	s.Lock()
	if _, ok := s.conn[domain]; ok {
		connections = s.conn[domain]
	}
	s.Unlock()
	if connections == nil || len(connections) == 0 {
		err = fmt.Errorf("no connections available for domain %s", domain)
		log.Debug(err)
		return
	}
	log.Debugf("requesting %s/%s from %d connections", domain, filePath, len(connections))

	// any connection that initated with this key is viable
	key := connections[0].Key

	// loop through connections randomly and try to get one to serve the file
	for _, i := range rand.Perm(len(connections)) {
		var p wsconn.Payload
		p, err = func() (p wsconn.Payload, err error) {
			err = connections[i].ws.Send(wsconn.Payload{
				Type:      "get",
				Message:   filePath,
				IPAddress: ipAddress,
			})
			if err != nil {
				return
			}
			p, err = connections[i].ws.Receive()
			return
		}()
		if err != nil {
			log.Debug(err)
			s.dumpConnection(domain, connections[i].ID)
			continue
		}
		log.Tracef("recv: %+v", p)
		if p.Type == "get" && p.Key == key {
			payload = p.Message
			if !p.Success {
				err = fmt.Errorf(payload)
			}
			return
		}
		log.Debugf("no good data from %d", i)
	}
	err = fmt.Errorf("invalid response")
	return
}

func (s *server) dumpConnection(domain string, id int) (err error) {
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

func remove(slice []*connection, s int) []*connection {
	return append(slice[:s], slice[s+1:]...)
}
