package main

import (
	"log"
	"net"
	"time"
)

func main() {
	// HTTP GET 请求测试
	log.Println("=== HTTP GET / ===")
	conn, err := net.DialTimeout("tcp", "smart120.p.pjservice.cn:8888", 5*time.Second)
	if err != nil {
		log.Fatalf("TCP dial failed: %v", err)
	}
	defer conn.Close()

	req := "GET / HTTP/1.1\r\n" +
		"Host: smart120.p.pjservice.cn:8888\r\n" +
		"User-Agent: GoTest\r\n" +
		"Accept: */*\r\n" +
		"\r\n"

	log.Printf("send:\n%s", req)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(req)); err != nil {
		log.Fatalf("write: %v", err)
	}
	log.Println("sent")

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("read: %v", err)
		if n > 0 {
			log.Printf("partial: %q", string(buf[:n]))
		}
	} else {
		log.Printf("response (%d bytes):\n%s", n, string(buf[:n]))
	}

	// 尝试读更多
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n2, err2 := conn.Read(buf)
	if err2 != nil {
		log.Printf("read2: %v", err2)
		if n2 > 0 {
			log.Printf("more data: %q", string(buf[:n2]))
		}
	} else {
		log.Printf("more data: %q", string(buf[:n2]))
	}
}
