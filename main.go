package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"github.com/h2non/filetype"
	log "github.com/schollz/logger"
	"github.com/urfave/cli"
	"github.com/vincent-petithory/dataurl"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

var Version string

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	app := cli.NewApp()
	app.Name = "hostyoself"
	if Version == "" {
		Version = "v0.0.0"
	}
	app.Version = Version
	app.Compiled = time.Now()
	app.Usage = "host your files using websockets from the command line or a browser"
	app.UsageText = "use to transfer files or host a impromptu website"
	app.Commands = []cli.Command{
		{
			Name:        "relay",
			Usage:       "start a relay",
			Description: "relay is used to transit files",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "url, u", Value: "localhost", Usage: "public URL to use"},
				cli.StringFlag{Name: "port", Value: "8010", Usage: "ports of the local relay"},
			},
			HelpName: "hostyoself relay",
			Action: func(c *cli.Context) error {
				return relay(c)
			},
		},
		{
			Name:        "host",
			Description: "host files from your computer",
			HelpName:    "hostyoself relay",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "url, u", Value: "https://hostyoself.com", Usage: "URL of relay to connect"},
			},
			Action: func(c *cli.Context) error {
				return host(c)
			},
		},
	}
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "debug", Usage: "increase verbosity"},
	}
	app.EnableBashCompletion = true
	app.HideHelp = false
	app.HideVersion = false
	app.BashComplete = func(c *cli.Context) {
		fmt.Fprintf(c.App.Writer, "host\nrelay")
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Debug(err)
	}
}

func host(c *cli.Context) (err error) {
	if c.GlobalBool("debug") {
		log.SetLevel("debug")
	} else {
		log.SetLevel("info")
	}

	return
}

func relay(c *cli.Context) (err error) {
	if c.GlobalBool("debug") {
		log.SetLevel("debug")
	} else {
		log.SetLevel("info")
	}

	flagPublicURL := c.String("url")
	if flagPublicURL == "localhost" {
		flagPublicURL += ":" + c.String("port")
	}
	if !strings.HasPrefix(flagPublicURL, "http") {
		flagPublicURL = "http://" + flagPublicURL
	}

	s := &server{
		port:      c.String("port"),
		conn:      make(map[string][]*Connection),
		publicURL: flagPublicURL,
	}
	return s.serve()
}

//
// websocket implementation
//

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Connection determine what can be held
type Connection struct {
	ID      int
	Joined  time.Time
	Domain  string
	Key     string
	LastGet string
	ws      *WebsocketConn
}

// Payload lists the data exchanged
type Payload struct {
	Success   bool   `json:"success"`
	Type      string `json:"type,omitempty"`
	Message   string `json:"message,omitempty"`
	IPAddress string `json:"ip,omitempty"`
	Key       string `json:"key,omitempty"`
}

func (p Payload) String() string {
	b, _ := json.Marshal(p)
	return string(b)
}

// WebsocketConn provides convenience functions for sending
// and receiving data with websockets, using mutex to
// make sure only one writer/reader
type WebsocketConn struct {
	ws *websocket.Conn
	sync.Mutex
}

// NewWebsocket returns a new websocket
func NewWebsocket(ws *websocket.Conn) *WebsocketConn {
	return &WebsocketConn{
		ws: ws,
	}
}

func (ws *WebsocketConn) Send(p Payload) (err error) {
	ws.Lock()
	defer ws.Unlock()
	log.Tracef("sending %+v", p)
	err = ws.ws.WriteJSON(p)
	return
}

func (ws *WebsocketConn) Receive() (p Payload, err error) {
	ws.Lock()
	defer ws.Unlock()
	err = ws.ws.ReadJSON(&p)
	log.Tracef("recv %+v", p)
	return
}

//
// server implementation
//

type server struct {
	publicURL string
	port      string

	// connections stored as map of domain -> connections
	conn map[string][]*Connection
	sync.Mutex
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
		// get IP address
		var ipAddress string
		ipAddress, err = GetClientIPHelper(r)
		if err != nil {
			log.Debugf("could not determine ip: %s", err.Error())
		}

		log.Debugf("attempting to find %s", r.URL.Path)

		// determine file path and the domain
		pathToFile := r.URL.Path[1:]
		domain := strings.Split(r.URL.Path[1:], "/")[0]
		// if there is a referer, try to obtain the domain from referer
		piecesOfReferer := strings.Split(r.Referer(), "/")
		if len(piecesOfReferer) > 4 && strings.HasPrefix(r.Referer(), s.publicURL) {
			domain = piecesOfReferer[3]
		}
		// prefix the domain if it doesn't exist
		if !strings.HasPrefix(pathToFile, domain) {
			pathToFile = domain + "/" + pathToFile
			http.Redirect(w, r, "/"+pathToFile, 302)
			return
		}
		// trim prefix to get the path to file
		pathToFile = strings.TrimPrefix(pathToFile, domain+"/")

		// send GET request to websockets
		var data string
		data, err = s.get(domain, pathToFile, ipAddress)
		if err != nil {
			// try index.html if it doesn't exist
			if string(pathToFile[len(pathToFile)-1]) != "/" {
				pathToFile += "/"
			}
			pathToFile += "index.html"
			data, err = s.get(domain, pathToFile, ipAddress)
			if err != nil {
				return
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

		// write the data to the requester
		w.Header().Set("Content-Type", contentType)
		w.Write(dataURL.Data)
		return
	}
	return
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

	if !(p.Type == "domain" && p.Message != "" && p.Key != "") {
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
		Key:    p.Key,
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

func (s *server) get(domain, filePath, ipAddress string) (payload string, err error) {

	var connections []*Connection
	s.Lock()
	if _, ok := s.conn[domain]; ok {
		connections = s.conn[domain]
	}
	s.Unlock()
	if connections == nil {
		err = fmt.Errorf("no connections available for domain %s", domain)
		log.Debug(err)
		return
	}
	log.Debugf("requesting %s/%s from %d connections", domain, filePath, len(connections))

	// any connection that initated with this key is viable
	key := connections[0].Key

	// loop through connections randomly and try to get one to serve the file
	for _, i := range rand.Perm(len(connections)) {
		var p Payload
		p, err = func() (p Payload, err error) {
			err = connections[i].ws.Send(Payload{
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
			s.DumpConnection(domain, connections[i].ID)
			continue
		}
		if len(p.Message) > 10 {
			p.Message = p.Message[:10] + "..."
		}
		log.Debugf("recv: %+v", p)
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

//
// client code
//

type Client struct {
	WebsocketURL string
	Domain       string
	Key          string
}

func (c *Client) Run() (err error) {
	log.Debugf("dialing %s", c.WebsocketURL)
	wsDial, _, err := websocket.DefaultDialer.Dial(c.WebsocketURL, nil)
	if err != nil {
		log.Error(err)
		return
	}
	defer wsDial.Close()

	ws := NewWebsocket(wsDial)

	err = ws.Send(Payload{
		Type:    "domain",
		Message: c.Domain,
		Key:     c.Key,
	})
	if err != nil {
		log.Error(err)
		return
	}

	for {
		var p Payload
		p, err = ws.Receive()
		if err != nil {
			log.Debug(err)
			return
		}
		log.Debugf("recv: %+v", p)

	}

	return
}

//
// utility functions
//

// GetClientIPHelper gets the client IP using a mixture of techniques.
// This is how it is with golang at the moment.
func GetClientIPHelper(req *http.Request) (ipResult string, errResult error) {

	// Try lots of ways :) Order is important.
	// Try Request Headers (X-Forwarder). Client could be behind a Proxy
	ip, err := getClientIPByHeaders(req)
	if err == nil {
		// log.Printf("debug: Found IP using Request Headers sniffing. ip: %v", ip)
		return ip, nil
	}

	// Try by Request
	ip, err = getClientIPByRequestRemoteAddr(req)
	if err == nil {
		// log.Printf("debug: Found IP using Request sniffing. ip: %v", ip)
		return ip, nil
	}

	//  Try Request Header ("Origin")
	url, err := url.Parse(req.Header.Get("Origin"))
	if err == nil {
		host := url.Host
		ip, _, err := net.SplitHostPort(host)
		if err == nil {
			// log.Printf("debug: Found IP using Header (Origin) sniffing. ip: %v", ip)
			return ip, nil
		}
	}

	err = errors.New("error: Could not find clients IP address")
	return "", err
}

// getClientIPByRequest tries to get directly from the Request.
// https://blog.golang.org/context/userip/userip.go
func getClientIPByRequestRemoteAddr(req *http.Request) (ip string, err error) {

	// Try via request
	ip, _, err = net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		// log.Printf("debug: Getting req.RemoteAddr %v", err)
		return "", err
	} else {
		// log.Printf("debug: With req.RemoteAddr found IP:%v; Port: %v", ip, port)
	}

	userIP := net.ParseIP(ip)
	if userIP == nil {
		message := fmt.Sprintf("debug: Parsing IP from Request.RemoteAddr got nothing.")
		// log.Printf(message)
		return "", fmt.Errorf(message)

	}
	// log.Printf("debug: Found IP: %v", userIP)
	return userIP.String(), nil

}

// getClientIPByHeaders tries to get directly from the Request Headers.
// This is only way when the client is behind a Proxy.
func getClientIPByHeaders(req *http.Request) (ip string, err error) {

	// Client could be behid a Proxy, so Try Request Headers (X-Forwarder)
	ipSlice := []string{}

	ipSlice = append(ipSlice, req.Header.Get("X-Forwarded-For"))
	ipSlice = append(ipSlice, req.Header.Get("x-forwarded-for"))
	ipSlice = append(ipSlice, req.Header.Get("X-FORWARDED-FOR"))

	for _, v := range ipSlice {
		// log.Printf("debug: client request header check gives ip: %v", v)
		if v != "" {
			return v, nil
		}
	}
	err = errors.New("error: Could not find clients IP address from the Request Headers")
	return "", err

}
