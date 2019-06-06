package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
)

// fanout.go: A simple unidirectional WS message fanout
// Subscribes to a single WS source and broadcasts each
// incoming message to every connected peer.

// TODO Ping health checks

const maxPressure = 10

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		logrus.Fatal("$PORT not set")
	}

	sourceUrl := os.Getenv("WS_SOURCE")
	if sourceUrl == "" {
		logrus.Fatal("$WS_SOURCE not set")
	}

	source := make(chan []byte)
	newConns := make(chan *websocket.Conn)

	go manage(source, newConns)

	// Connect to source and ingest messages
	go receiver(sourceUrl, source)

	// Collect WS connections
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		wsHandler(w, r, newConns)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		logrus.Fatal(err)
	}
}

// receiver dumps messages from sourceUrl into incoming.
// Kills process if connection fails.
func receiver(sourceUrl string, incoming chan<- []byte) {
	defer close(incoming)

	source, _, err := websocket.DefaultDialer.Dial(sourceUrl, nil)
	if err != nil {
		logrus.Fatal(err)
	}

	for {
		msgType, msg, err := source.ReadMessage()
		if err != nil {
			logrus.Fatal(err)
		}
		if msgType != websocket.TextMessage {
			logrus.Warn("Ignoring incoming non-text message")
			continue
		}
		incoming <- msg
	}
}

// acceptor dumps an upgraded connection into conns
func wsHandler(w http.ResponseWriter, r *http.Request, conns chan<- *websocket.Conn) {
	// Upgrade connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.WithError(err).Warn("Failed to upgrade WS")
		return
	}
	logrus.WithField("addr", r.RemoteAddr).Info("New connection")
	conns <- conn
}

func manage(source <-chan []byte, newConns chan *websocket.Conn) {
	m := Manager{
		source: source,
		newConns: newConns,
	}
	m.run()
}
