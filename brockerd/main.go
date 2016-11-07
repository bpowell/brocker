package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Service struct {
	Name       string `json:"name"`
	BridgeName string `json:"bridge-name"`
	BridgeIP   string `json:"bridge-ip"`
	ServicePid int
}

type Container struct {
	Name        string `json:"name"`
	ServiceName string `json:"service-name"`
	Command     string `json:"command"`
	Pid         int
	IP          string
	StartTime   time.Time
}

var services map[string]Service
var containers []Containter

func init() {
	services = make(map[string]Service)
}

func main() {
	http.HandleFunc("/api/v1/service/add", service_add)
	http.HandleFunc("/api/v1/container/run", container_run)
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

	var s Service
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, ok := services[s.Name]; ok {
		http.Error(w, "Service already exists", http.StatusInternalServerError)
		return
	}

	if err := service_create_network(s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func container_run(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid Request!", http.StatusMethodNotAllowed)
		return
	}

	var c Container
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, ok := services[c.ServiceName]; ok == false {
		http.Error(w, "Service does not exists", http.StatusInternalServerError)
		return
	}

	go run(c)

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

func run(c Container) {
	fmt.Println("running parent")
	runcmd := "/home/yup/p/containers/brocker-run/brocker-run"

	cmd := &exec.Cmd{
		Path: runcmd,
		Args: append([]string{runcmd}, c.Command),
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET,
	}

	if err := cmd.Start(); err != nil {
		fmt.Println(err)
	}

	c.Pid = cmd.Process.Pid
	c.StartTime = time.Now()
	containers = append(containers, c)
	fmt.Println(cmd.Process.Pid)

	cmd.Wait()
}
