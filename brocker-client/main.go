package main

import (
	"io"
	"log"
	"net"
	"os"
	"strings"
)

func reader(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf[:])
		if err != nil {
			return
		}
		println("Client got:", string(buf[0:n]))
	}
}

func main() {
	c, err := net.Dial("unix", "/tmp/container.sock")
	if err != nil {
		panic(err)
	}
	defer c.Close()

	go reader(c)
	if os.Args[1] == "run" {
		_, err = c.Write([]byte(strings.Join(os.Args[1:], " ")))
		if err != nil {
			log.Fatal("write error:", err)
		}
	} else if os.Args[1] == "exec" {
		q, err := net.Dial("unix", "/tmp/command.sock")
		if err != nil {
			panic(err)
		}
		defer q.Close()
		_, err = q.Write([]byte(strings.Join(os.Args[1:], " ")))
		if err != nil {
			log.Fatal("write error:", err)
		}
	}
}
