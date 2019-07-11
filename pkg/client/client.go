package client

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/schollz/hostyoself/pkg/namesgenerator"
	"github.com/schollz/hostyoself/pkg/utils"
	"github.com/schollz/hostyoself/pkg/wsconn"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	log "github.com/schollz/logger"
	"github.com/vincent-petithory/dataurl"
)

type client struct {
	WebsocketURL string
	Domain       string
	Key          string
	Folder       string
	fileList     map[string]struct{}
	sync.Mutex
}

// New returns a new client
func New(domain, key, webocketURL, folder string) (c *client, err error) {
	if strings.HasPrefix(webocketURL, "http") {
		webocketURL = strings.Replace(webocketURL, "http", "ws", 1)
	}
	webocketURL += "/ws"

	if domain == "" {
		domain = namesgenerator.GetRandomName()
	}

	if key == "" {
		key = utils.RandStringBytesMaskImpr(6)
	}

	if folder == "" {
		folder = "."
	}

	folder, _ = filepath.Abs(folder)
	folder = filepath.ToSlash(folder)

	if _, err = os.Stat(folder); os.IsNotExist(err) {
		log.Error(err)
		return
	}

	log.Infof("connecting to %s", webocketURL)
	log.Infof("using domain '%s'", domain)
	log.Infof("using key '%s'", key)
	log.Infof("watching folder '%s'", folder)

	c = &client{
		WebsocketURL: webocketURL,
		Domain:       domain,
		Key:          key,
		Folder:       folder,
		fileList:     make(map[string]struct{}),
	}
	return
}

func (c *client) Run() (err error) {
	go c.watchFileSystem()

	log.Debugf("dialing %s", c.WebsocketURL)
	wsDial, _, err := websocket.DefaultDialer.Dial(c.WebsocketURL, nil)
	if err != nil {
		log.Error(err)
		return
	}
	defer wsDial.Close()

	ws := wsconn.New(wsDial)

	err = ws.Send(wsconn.Payload{
		Type:    "domain",
		Message: c.Domain,
		Key:     c.Key,
	})
	if err != nil {
		log.Error(err)
		return
	}

	for {
		var p wsconn.Payload
		p, err = ws.Receive()
		if err != nil {
			log.Debug(err)
			return
		}
		log.Debugf("recv: %+v", p)

		if p.Type == "get" {
			haveFile := false
			c.Lock()
			_, haveFile = c.fileList[p.Message]
			c.Unlock()
			if !haveFile {
				err = ws.Send(wsconn.Payload{
					Type:    "get",
					Success: false,
					Message: "no such file",
					Key:     c.Key,
				})
				log.Infof("%s /%s 404", p.IPAddress, p.Message)
			} else {
				var b []byte

				b, err = ioutil.ReadFile(path.Join(c.Folder, p.Message))
				if err != nil {
					log.Error(err)
					return
				}
				err = ws.Send(wsconn.Payload{
					Type:    "get",
					Success: true,
					Message: dataurl.EncodeBytes(b),
					Key:     c.Key,
				})
				log.Infof("%s /%s 200", p.IPAddress, p.Message)
			}
		}
		if err != nil {
			log.Debug(err)
			return
		}

	}

	return
}

func (c *client) watchFileSystem() (err error) {
	// creates a new file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Debugf("event: [%s] [%s]", event.Name, strings.ToLower(event.Op.String()))
				c.Lock()
				switch strings.ToLower(event.Op.String()) {
				case "create":
					c.fileList[filepath.ToSlash(event.Name)] = struct{}{}
				case "remove":
					delete(c.fileList, filepath.ToSlash(event.Name))
				}
				log.Debugf("map: %+v", c.fileList)
				c.Unlock()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error("error:", err)
			}
		}
	}()

	filepath.Walk(c.Folder, func(ppath string, fi os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("problem with '%s': %s", ppath, err.Error())
			return err
		}
		ppath = filepath.ToSlash(ppath)
		if strings.Contains(ppath, ".git") {
			return nil
		}
		if fi.Mode().IsDir() {
			log.Debugf("watching %s", ppath)
			return watcher.Add(ppath)
		} else {
			ppath, _ = filepath.Abs(ppath)
			ppath = strings.TrimPrefix(filepath.ToSlash(ppath), c.Folder+"/")
			c.Lock()
			c.fileList[ppath] = struct{}{}
			c.Unlock()
		}
		return nil
	})

	<-done
	return
}
