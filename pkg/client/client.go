package client

import "github.com/gorilla/websocket"

type client struct {
	WebsocketURL string
	Domain       string
	Key          string
}

func New(domain, key, webocketURL string) *client {
	return &client{
		WebsocketURL: webocketURL,
		Domain:       domain,
		Key:          key,
	}
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
