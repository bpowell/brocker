package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type Service struct {
	Name       string
	BridgeName string
	BridgeIP   net.IP
	ServicePid int
}

var services map[string]Service

func init() {
	services = make(map[string]Service)
}

func main() {
	http.HandleFunc("/api/v1/service/add", service_add)
	err := http.ListenAndServe(":3000", nil)
	if err != nil {
		fmt.Println(err)
	}
}

func service_add(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid Request!", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	var s Service
	s.Name = r.PostFormValue("name")
	if _, ok := services[s.Name]; ok {
		http.Error(w, "Service already exists", http.StatusInternalServerError)
		return
	}

	s.BridgeName = r.PostFormValue("bridge-name")
	s.BridgeIP = net.ParseIP(r.PostFormValue("bridge-ip"))

	err := service_create_network(s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func service_create_network(s Service) error {
	create_bridge := strings.Split(fmt.Sprintf("/sbin/ip link add name %s type bridge", s.BridgeName), " ")
	set_bridge_up := strings.Split(fmt.Sprintf("/sbin/ip link set %s up", s.BridgeName), " ")
	set_bridge_ip := strings.Split(fmt.Sprintf("/sbin/ifconfig %s %s", s.BridgeName, s.BridgeIP), " ")

	cmd1 := exec.Command(create_bridge[0], create_bridge[1:]...)
	err := cmd1.Run()
	if err != nil {
		return err
	}

	cmd2 := exec.Command(set_bridge_up[0], set_bridge_up[1:]...)
	err = cmd2.Run()
	if err != nil {
		return err
	}

	cmd3 := exec.Command(set_bridge_ip[0], set_bridge_ip[1:]...)
	err = cmd3.Run()
	if err != nil {
		return err
	}

	services[s.Name] = s
	return nil
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
	runcmd := "/home/yup/p/containers/brocker-run/brocker-run"

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
