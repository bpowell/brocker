package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Service struct {
	Name       string `json:"name"`
	BridgeName string
	BridgeIP   string `json:"bridge-ip"`
	NginxConf  string `json:"nginx-config"`
	Containers []Container
}

type Container struct {
	Name        string `json:"name"`
	ServiceName string `json:"service-name"`
	Command     string `json:"command"`
	Pid         int
	IP          string
	StartTime   time.Time
	VEth        string
}

var services map[string]Service
var containers []Container

const (
	bridgeNameBase = "brocker"
	vethNameBase   = "veth"
)

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

	if _, err := os.Stat(s.NginxConf); os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("Cannot open %s\n%s", s.NginxConf, err.Error()), http.StatusInternalServerError)
		return
	}

	s.BridgeName = fmt.Sprintf("%s%d", bridgeNameBase, len(services)+1)

	if err := service_create_network(s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	path, err := exec.LookPath("nginx")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c := Container{
		Name:        fmt.Sprintf("%s-nginx", s.Name),
		ServiceName: s.Name,
		Command:     fmt.Sprintf("%s -c %s", path, s.NginxConf),
	}
	go run(c)

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

	if err := exec.Command(create_bridge[0], create_bridge[1:]...).Run(); err != nil {
		return err
	}

	if err := exec.Command(set_bridge_up[0], set_bridge_up[1:]...).Run(); err != nil {
		return err
	}

	if err := exec.Command(set_bridge_ip[0], set_bridge_ip[1:]...).Run(); err != nil {
		return err
	}

	services[s.Name] = s
	return nil
}

func run(c Container) {
	fmt.Println("running parent")
	s := services[c.ServiceName]
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
	c.VEth = fmt.Sprintf("%s%d", vethNameBase, len(containers))
	link := strings.Split(fmt.Sprintf("/sbin/ip link add name %s type veth peer name veth1 netns %d", c.VEth, c.Pid), " ")
	if err := exec.Command(link[0], link[1:]...).Run(); err != nil {
		fmt.Println(err)
		return
	}

	uplink := strings.Split(fmt.Sprintf("/sbin/ifconfig %s up", c.VEth), " ")
	if err := exec.Command(uplink[0], uplink[1:]...).Run(); err != nil {
		fmt.Println(err)
		return
	}

	bridge := strings.Split(fmt.Sprintf("/sbin/ip link set %s master %s", c.VEth, s.BridgeName), " ")
	if err := exec.Command(bridge[0], bridge[1:]...).Run(); err != nil {
		fmt.Println(err)
		return
	}

	bridgeip := net.ParseIP(s.BridgeIP)
	lastOctet := bridgeip[15] + byte(len(s.Containers)+1)
	ip := net.IPv4(bridgeip[12], bridgeip[13], bridgeip[14], lastOctet)
	c.IP = ip.String()

	if err := execInContainter(fmt.Sprintf("/sbin/ifconfig veth1 %s", ip.String()), c.Pid); err != nil {
		fmt.Println(err)
		return
	}

	c.StartTime = time.Now()
	containers = append(containers, c)

	s.Containers = append(s.Containers, c)
	services[c.ServiceName] = s

	fmt.Println(cmd.Process.Pid)

	cmd.Wait()
}

func execInContainter(cmd string, pid int) error {
	command := strings.Split(fmt.Sprintf("nsenter --target %d --pid --net %s", pid, cmd), " ")
	if err := exec.Command(command[0], command[1:]...).Run(); err != nil {
		return err
	}

	return nil
}
