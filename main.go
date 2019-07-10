package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

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

	s := server.New(flagPublicURL, c.String("port"))
	return s.Run()
}
