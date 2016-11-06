package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func main() {
	l, err := net.Listen("unix", "/tmp/container.sock")
	if err != nil {
		log.Fatal("listen error:", err)
	}

	for {
		fd, err := l.Accept()
		if err != nil {
			log.Fatal("accept error:", err)
		}

		go echoServer(fd)
	}
}

func echoServer(c net.Conn) {
	for {
		buf := make([]byte, 512)
		nr, err := c.Read(buf)
		if err != nil {
			return
		}

		data := buf[0:nr]
		raw := strings.Split(string(data), " ")

		println("Server got:", string(data))
		if raw[0] == "run" {
			parent(raw[1:])
		}
	}
}

func parent(args []string) {
	fmt.Println("running parent")
	runcmd := "/home/yup/p/containers/run/run"

	cmd := &exec.Cmd{
		Path: runcmd,
		Args: append([]string{runcmd}, args...),
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET,
	}

	if err := cmd.Start(); err != nil {
		return
	}

	fmt.Println(cmd.Process.Pid)

	cmd.Wait()
}
