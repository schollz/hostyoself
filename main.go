package main

//go:generate go get -v github.com/jteeuwen/go-bindata/go-bindata
//go:generate go-bindata -pkg server -o pkg/server/assets.go templates/ static/

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/schollz/hostyoself/pkg/client"
	"github.com/schollz/hostyoself/pkg/server"
	log "github.com/schollz/logger"
	"github.com/urfave/cli"
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
	app.UsageText = "use to transfer files or host an impromptu website"
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
			Usage:       "host files from your computer",
			Description: "host files from your computer",
			HelpName:    "hostyoself relay",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "url, u", Value: "https://hostyoself.com", Usage: "URL of relay to connect"},
				cli.StringFlag{Name: "domain, d", Value: "", Usage: "domain to use (default is random)"},
				cli.StringFlag{Name: "key, k", Value: "", Usage: "key value to use (default is random)"},
				cli.StringFlag{Name: "folder, f", Value: ".", Usage: "folder to serve files"},
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

	cl, err := client.New(c.String("domain"), c.String("key"), c.String("url"), c.String("folder"))
	if err != nil {
		return
	}
	for {
		log.Info("serving forever")
		err = cl.Run()
		if err != nil {
			log.Debug(err)
		}
		log.Infof("server disconnected, retrying in 10 seconds")
		time.Sleep(10 * time.Second)
	}
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

	s := server.New(flagPublicURL, c.String("port"))
	return s.Run()
}
