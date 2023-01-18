package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type client struct {
	nickname string
	channel  string
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

func send(conn net.Conn, msg string) bool {
	_, err := conn.Write([]byte(msg))
	if err != nil {
		if err == io.EOF {
			fmt.Printf("%v Connection closed\n", conn.RemoteAddr())
			return false
		}
		fmt.Printf("%v Error: %v\n", conn.RemoteAddr(), err)
		return false
	}
	return true
}

func setNickname(conn net.Conn, cmd []string) {
	mu.Lock()
	c := clients[conn.RemoteAddr().String()]

	if !existsParam(cmd) {
		msg := fmt.Sprintf("No channel name\n")
		if !send(c.conn, msg) {
			mu.Unlock()
			return
		}

		mu.Unlock()
		return
	}

	if existsNickname(cmd[1]) {
		msg := fmt.Sprintf("%v is already used\n", cmd[1])
		if !send(conn, msg) {
			mu.Unlock()
			return
		}

		mu.Unlock()
		return
	}

	oldNick := c.nickname
	c.nickname = cmd[1]
	clients[conn.RemoteAddr().String()] = c
	msg := fmt.Sprintf("NICKNAME: %v changed nickname to %v", oldNick, cmd[1])
	for _, c := range clients {
		if !send(c.conn, msg) {
			mu.Unlock()
			return
		}
	}

	mu.Unlock()
}

func joinChannel(conn net.Conn, cmd []string) {
	mu.Lock()
	c := clients[conn.RemoteAddr().String()]

	if !existsParam(cmd) {
		msg := fmt.Sprintf("No channel name\n")
		if !send(c.conn, msg) {
			mu.Unlock()
			return
		}

		mu.Unlock()
		return
	}

	c.channel = cmd[1]
	clients[conn.RemoteAddr().String()] = c
	for key, c := range clients {
		msg := fmt.Sprintf("JOIN: %v joined in the channel %v\n", c.nickname, c.channel)
		if clients[key].channel == cmd[1] {
			if !send(c.conn, msg) {
				mu.Unlock()
				return
			}
		}
	}

	mu.Unlock()
}

func partChannel(conn net.Conn, client *client) {
	mu.Lock()
	c := clients[conn.RemoteAddr().String()]
	oldChannel := c.channel
	c.channel = ""
	clients[conn.RemoteAddr().String()] = c

	if oldChannel == "" {
		msg := fmt.Sprintf("PART: %v are not in a channel\n", client.nickname)
		if !send(c.conn, msg) {
			mu.Unlock()
			return
		}
	} else {
		for key, c := range clients {
			msg := fmt.Sprintf("PART: %v left from the channel %v\n", client.nickname, client.channel)
			if clients[key].channel == oldChannel {
				if !send(c.conn, msg) {
					mu.Unlock()
					return
				}
			}
		}
	}

	mu.Unlock()
}

func existsParam(cmd []string) bool {
	return len(cmd) > 1
}

func existsNickname(nickname string) bool {
	for key := range clients {
		if clients[key].nickname == nickname {
			return true
		}
	}

	return false
}

func handler(conn net.Conn) {
	pingInterval := time.Second * 5
	maxPingInterval := time.Second * 15
	msgReadCh := make(chan string)
	errCh := make(chan error)
	lastMsgTime := time.Now()

	clientInstance := &client{
		conn:     conn,
		nickname: "unknown: " + conn.RemoteAddr().String(),
		channel:  "",
	}

	addClient(conn.RemoteAddr().String(), clientInstance)

	defer func() {
		close(msgReadCh)
		close(errCh)
		conn.Close()
		removeClient(conn.RemoteAddr().String())
	}()

	go handleReadConn(conn, msgReadCh, errCh)

	for {
		select {
		case <-time.After(pingInterval):
			if time.Since(lastMsgTime) > pingInterval {
				if !send(conn, "ping\n") {
					return
				}
			}
			if time.Since(lastMsgTime) > maxPingInterval {
				fmt.Println("Inactive connection, closing")
				return
			}

		case msg := <-msgReadCh:
			lastMsgTime = time.Now()
			cmd := strings.Split(strings.TrimSpace(msg), " ")

			command := cmd[0]

			switch command {
			case "pong":
				continue

			case "NICK":
				setNickname(conn, cmd)

				continue
			case "JOIN":
				c := clients[conn.RemoteAddr().String()]

				if c.channel == "" {
					joinChannel(conn, cmd)
				} else {
					partChannel(conn, clientInstance)
					joinChannel(conn, cmd)
				}

				continue
			case "PART":
				partChannel(conn, clientInstance)

				continue
			case "LIST":
				mu.Lock()

				var list string
				channels := make(map[string]int)

				for _, c := range clients {
					if c.channel != "" {
						channels[c.channel]++
					}
				}

				for key, value := range channels {
					list += key + ": " + strconv.Itoa(value) + "\n"
				}

				msg := fmt.Sprintf("\n%v", list)
				if !send(clientInstance.conn, msg) {
					mu.Unlock()
					return
				}

				mu.Unlock()

				continue
			case "PRIVMSG":
				mu.Lock()

				if !existsParam(cmd) {
					msg := fmt.Sprintf("No channel name\n")
					if !send(clientInstance.conn, msg) {
						mu.Unlock()
						return
					}

					mu.Unlock()
					return
				}

				if clientInstance.channel == cmd[1] {
					for _, c := range clients {
						if clientInstance.channel == c.channel {
							msg := fmt.Sprintf("%v: %v\n", clientInstance.nickname, msg)
							if !send(c.conn, msg) {
								mu.Unlock()
								return
							}
						}
					}
				} else {
					for _, c := range clients {
						if c.nickname == cmd[1] {
							msg := fmt.Sprintf("%v: %v\n", clientInstance.nickname, strings.Join(cmd[2:], " "))
							if !send(c.conn, msg) {
								mu.Unlock()
								return
							}
							break
						}
					}
				}

				mu.Unlock()

				continue
			case "WHO":
				mu.Lock()
				c := clients[conn.RemoteAddr().String()]

				if !existsParam(cmd) {
					msg := fmt.Sprintf("No channel name\n")
					if !send(c.conn, msg) {
						mu.Unlock()
						return
					}
				} else {
					var list string

					for _, value := range clients {
						if value.channel == cmd[1] {
							list += value.nickname + "\n"
						}
					}

					msg := fmt.Sprintf("%v", list)
					if !send(clientInstance.conn, msg) {
						mu.Unlock()
						return
					}
				}

				mu.Unlock()

				continue
			case "QUIT":
				mu.Lock()
				for key, c := range clients {
					msg := fmt.Sprintf("PART: %v left from the channel %v\n", clientInstance.nickname, clientInstance.channel)
					if clients[key].channel == clientInstance.channel {
						if !send(c.conn, msg) {
							mu.Unlock()
							return
						}
					}
				}

				mu.Unlock()
				continue
			case "":
				continue
			default:
				mu.Lock()

				msg := fmt.Sprintf("ERR UNKNOWNCOMMAND\n")
				if !send(clientInstance.conn, msg) {
					mu.Unlock()
					return
				}

				mu.Unlock()

				continue
			}

		case err := <-errCh:
			if err == io.EOF {
				fmt.Printf("%v Connection closed\n", conn.RemoteAddr())
			}
			fmt.Println("Error reading from connection", err)
			return
		}
	}
}

func main() {
	fmt.Println("Listening on port 8888")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Println("\nShutdown server...")
		os.Exit(0)
	}()

	ln, _ := net.Listen("tcp", ":8888")

	for {
		conn, _ := ln.Accept()
		fmt.Printf("%v Connection accepted\n", conn.RemoteAddr())
		go handler(conn)
	}
}
