package wsconn

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/schollz/logger"
)

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
func New(ws *websocket.Conn) *WebsocketConn {
	return &WebsocketConn{
		ws: ws,
	}
}

func (ws *WebsocketConn) Close() (err error) {
	ws.Lock()
	defer ws.Unlock()
	ws.Close()
	ws = nil
	return
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
