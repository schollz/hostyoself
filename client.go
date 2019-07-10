package main

import (
	"github.com/gorilla/websocket"
	log "github.com/schollz/logger"
)

type Client struct {
	WebsocketURL string
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
		Type:"domain",
		Message:"zack",
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
