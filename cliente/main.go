package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	output    = make(chan string)
	input     = make(chan string)
	errorChan = make(chan error)
)

func readStdin() {
	for {
		reader := bufio.NewReader(os.Stdin)
		m, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}

		input <- m
	}
}

func readConn(conn net.Conn) {
	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			errorChan <- err
			return
		}

		m := string(buf[:n])
		output <- m
	}
}

func send(conn net.Conn, m string) {
	if conn == nil {
		return
	}

	_, err := conn.Write([]byte(m))
	if err != nil {
		fmt.Println(err)
		conn.Close()
		conn = nil
	}
}

func main() {

	var conn net.Conn
	go readStdin()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigs:
			fmt.Println("\nDisconnecting...")
			os.Exit(0)

		case m := <-output:
			if m == "ping\n" {
				send(conn, "pong\n")
				continue
			}

			fmt.Print(m)

		case m := <-input:
			cmd := strings.Split(strings.TrimSpace(m), " ")
			command := cmd[0]

			switch command {
			case "/connect":
				server := cmd[1]
				var err error

				fmt.Printf("Connecting to %s...\n", server)
				conn, err = net.Dial("tcp", server)
				if err != nil {
					fmt.Printf("Error: %s\n", err)
					continue
				}

				fmt.Printf("Connected to %s\n", server)
				go readConn(conn)
				continue

			case "QUIT":
				send(conn, "QUIT\n")
				fmt.Println("\nDisconnecting...")
				os.Exit(0)

			case "":
				continue
			}

			if conn == nil {
				fmt.Println("Not connected")
				continue
			}

			send(conn, m)
		}
	}
}
