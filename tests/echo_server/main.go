package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("echo client connected: %s path=%s", r.RemoteAddr, r.URL.Path)

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("read error: %v", err)
			break
		}
		log.Printf("echo received (%d bytes): %s", len(msg), string(msg))
		if err := conn.WriteMessage(mt, msg); err != nil {
			log.Printf("write error: %v", err)
			break
		}
		log.Printf("echo sent back: %s", string(msg))
	}
}

func main() {
	http.HandleFunc("/", echoHandler)
	http.HandleFunc("/ws", echoHandler)
	addr := ":9090"
	fmt.Printf("echo server listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
