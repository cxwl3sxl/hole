package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	testType := "direct"
	if len(os.Args) > 1 {
		testType = os.Args[1]
	}

	switch testType {
	case "direct":
		testDirect()
	case "tunnel":
		testTunnel()
	default:
		fmt.Println("usage: go run . [direct|tunnel]")
	}
}

func testDirect() {
	// 直连回显服务
	u := url.URL{Scheme: "ws", Host: "localhost:9090", Path: "/ws"}
	log.Printf("direct connect to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("direct dial error: %v", err)
	}
	defer c.Close()
	log.Println("direct connected OK")

	testEcho(c)
}

func testTunnel() {
	// 通过隧道连接（模拟浏览器，Host 头设子域名）
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}

	// 关键：模拟 Nginx 设置 Host 头，让 megad 能提取子域名
	header := http.Header{}
	header.Set("Host", "s120.p.pjservice.cn:8080")

	log.Printf("tunnel connect to %s (Host: s120.p.pjservice.cn:8080)", u.String())

	c, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			log.Printf("tunnel dial failed, status=%d", resp.StatusCode)
			log.Printf("response headers:")
			for k, v := range resp.Header {
				log.Printf("  %s: %s", k, v)
			}
			body := make([]byte, 1024)
			resp.Body.Read(body)
			log.Printf("response body: %s", string(body))
		}
		log.Fatalf("tunnel dial error: %v", err)
	}
	defer c.Close()
	log.Println("tunnel connected OK")

	testEcho(c)
}

func testEcho(c *websocket.Conn) {
	// 发送消息
	msg := fmt.Sprintf("hello %s", time.Now().Format(time.RFC3339))
	err := c.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		log.Fatalf("write error: %v", err)
	}
	log.Printf("sent: %s", msg)

	// 读取回显
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, reply, err := c.ReadMessage()
	if err != nil {
		log.Fatalf("read error: %v", err)
	}
	log.Printf("received: %s", string(reply))

	if string(reply) != msg {
		log.Fatalf("echo mismatch: sent=%q recv=%q", msg, string(reply))
	}
	log.Println("echo test PASSED")
}
