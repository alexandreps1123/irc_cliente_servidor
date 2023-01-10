package main

import (
	"fmt"
	"net"
	"sync"
)

type client struct {
	nickname string
	conn     net.Conn
}

var (
	clients = make(map[string]*client, 0)
	mu      sync.Mutex
)

func removeClient(ip string) {
	mu.Lock()
	defer mu.Unlock()
	delete(clients, ip)
}

func addClient(ip string, c *client) {
	mu.Lock()
	defer mu.Unlock()
	clients[ip] = c
}

func handleReadConn(conn net.Conn, msgReadCh chan string, errCh chan error) {
	for {
		if msgReadCh == nil || conn == nil {
			return
		}

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			errCh <- err
			return
		}

		m := string(buf[:n])
		msgReadCh <- m
	}
}

func main() {
	fmt.Println("Listening on port 8888")

	ln, _ := net.Listen("tcp", ":8888")

	for {
		conn, _ := ln.Accept()
		fmt.Println("Connection accepted")
		go handler(conn)
	}
}
