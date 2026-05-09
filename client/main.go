package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"time"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var (
	background = false
	//go:embed config.json
	config          []byte
	contactMessages map[string][]QueueMessage
)

type QueueMessage struct {
	sent time.Time
	msg  string
}

type Config struct {
	Addr      string `json:"addr"`
	KeyStore  string `json:"keystore"`
	Endpoint  string `json:"endpoint"`
	ServerKey string `json:"serverkey"`
}

func main() {
	contactMessages = make(map[string][]QueueMessage)
	c := Config{}
	if err := json.Unmarshal(config, &c); err != nil {
		log.Fatalf("Error parsing config file: %v\n", err)
	}
	g := &GUI{
		serverKey:       c.ServerKey,
		scrollContainer: make(map[string]*container.Scroll),
		chatOutput:      make(map[string]*widget.RichText),
		enc: &Encryption{
			iter:           100000,
			configKeystore: []byte(c.KeyStore),
		},
		client: &Client{
			SelfSigned:  true,
			Addr:        c.Addr,
			wsPath:      c.Endpoint,
			MessageChan: make(chan []byte),
			serverKey:   c.ServerKey,
		},
		app:  app.New(),
		tabs: false,
	}
	g.client.enc = g.enc
	g.window = g.app.NewWindow("Login")
	g.window.SetMaster()
	platformDo(g)
	g.loginWindow()
	go g.listen()
	g.lifecycle()
	g.window.ShowAndRun()
}
