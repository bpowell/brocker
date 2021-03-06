package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/bpowell/brocker/container"
	"github.com/bpowell/brocker/service"
)

var services map[string]service.Service
var containers map[string]container.Container

const (
	bridgeNameBase = "brocker"
	vethNameBase   = "veth"
	MOUNT_LOC      = "/app"
	CONTAIN_DIR    = "/container"
)

func init() {
	services = make(map[string]service.Service)
	containers = make(map[string]container.Container)
}

func main() {
	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC, os.Interrupt)
	go func() {
		for range ctrlC {
			for _, s := range services {
				s.Stop()
			}
			os.Exit(0)
		}
	}()

	http.HandleFunc("/api/v1/service/add", serviceAdd)
	http.HandleFunc("/api/v1/container/run", containerRun)
	http.HandleFunc("/api/v1/container/list", containerList)
	http.HandleFunc("/api/v1/container/exec", containerExec)
	http.HandleFunc("/api/v1/container/stop", containerStop)
	err := http.ListenAndServe(":3000", nil)
	if err != nil {
		fmt.Println(err)
	}
}

func serviceAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid Request!", http.StatusMethodNotAllowed)
		return
	}

	s := service.Service{
		Containers: make(map[string]container.Container),
	}

	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, ok := services[s.Name]; ok {
		http.Error(w, "Service already exists", http.StatusInternalServerError)
		return
	}

	s.BridgeName = fmt.Sprintf("%s%d", bridgeNameBase, len(services)+1)
	s.NewIPPool()

	s.LoadBalanceType = "least_conn"
	if err := serviceCreateNetwork(s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	path, err := exec.LookPath("nginx")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c := container.Container{
		Name:        fmt.Sprintf("%s-nginx", s.Name),
		ServiceName: s.Name,
		Command:     fmt.Sprintf("%s -c %s", path, "/app/nginx.conf"),
	}

	go run(c, true)

	w.WriteHeader(http.StatusCreated)
}

func containerRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid Request!", http.StatusMethodNotAllowed)
		return
	}

	var c container.Container
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, ok := services[c.ServiceName]; ok == false {
		http.Error(w, "Service does not exists", http.StatusInternalServerError)
		return
	}

	go run(c, false)

	w.WriteHeader(http.StatusCreated)
}

func containerList(w http.ResponseWriter, r *http.Request) {
	files, err := ioutil.ReadDir(CONTAIN_DIR)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var all []container.Container
	for _, f := range files {
		raw, err := ioutil.ReadFile(fmt.Sprintf("%s/%s/config", CONTAIN_DIR, f.Name()))
		if err != nil {
			fmt.Println(err)
			continue
		}
		var c container.Container

		if err := json.Unmarshal(raw, &c); err != nil {
			fmt.Println(err)
			continue
		}

		all = append(all, c)
	}

	if err := json.NewEncoder(w).Encode(all); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func containerExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid Request!", http.StatusMethodNotAllowed)
		return
	}

	data := struct {
		Name string `json:"name"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, c := range containers {
		if c.Name == data.Name {
			w.Write([]byte(fmt.Sprintf("%d", c.Pid)))
			return
		}
	}
}

func containerStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid Request!", http.StatusMethodNotAllowed)
		return
	}

	data := struct {
		Name string `json:"name"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c, ok := containers[data.Name]
	if !ok {
		http.Error(w, "Not a running container", http.StatusInternalServerError)
		return
	}
	c.Close()

	w.Write([]byte("Stopping container"))
}

func serviceCreateNetwork(s service.Service) error {
	createBridge := strings.Split(fmt.Sprintf("/sbin/ip link add name %s type bridge", s.BridgeName), " ")
	setBridgeUp := strings.Split(fmt.Sprintf("/sbin/ip link set %s up", s.BridgeName), " ")
	setBridgeIP := strings.Split(fmt.Sprintf("/sbin/ifconfig %s %s", s.BridgeName, s.BridgeIP), " ")

	if err := exec.Command(createBridge[0], createBridge[1:]...).Run(); err != nil {
		return err
	}

	if err := exec.Command(setBridgeUp[0], setBridgeUp[1:]...).Run(); err != nil {
		return err
	}

	if err := exec.Command(setBridgeIP[0], setBridgeIP[1:]...).Run(); err != nil {
		return err
	}

	services[s.Name] = s
	return nil
}

func run(c container.Container, isNginx bool) {
	fmt.Println("running parent")
	s := services[c.ServiceName]
	runcmd, err := exec.LookPath("brocker-run")
	if err != nil {
		fmt.Println(err)
		return
	}

	c.StartTime = time.Now()
	c.SetName()
	c.Active = true
	c.Directory = fmt.Sprintf("%s/%s", CONTAIN_DIR, c.Name)
	c.VEth = fmt.Sprintf("%s%d", vethNameBase, len(containers))

	defer c.WriteConfig()

	if err := os.Mkdir(c.Directory, 0644); err != nil {
		fmt.Println(err)
		return
	}

	c.WriteConfig()

	stdouterr, err := os.Create(fmt.Sprintf("%s/out", c.Directory))
	if err != nil {
		fmt.Println("Cannot create out:", err)
		return
	}
	defer stdouterr.Close()

	if c.CopyFile {
		if err := exec.Command("cp", c.FileToCopy, fmt.Sprintf("%s/%s", c.Directory, path.Base(c.FileToCopy))).Run(); err != nil {
			fmt.Println(err)
			return
		}
	}

	if isNginx {
		s.ContainterName = c.Name
		s.WriteConfig(CONTAIN_DIR)
	}

	args := strings.Split(fmt.Sprintf("%s %s %s", runcmd, c.Name, c.Command), " ")

	cmd := &exec.Cmd{
		Path: runcmd,
		Args: args,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdouterr
	cmd.Stderr = stdouterr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET,
	}

	if err := cmd.Start(); err != nil {
		fmt.Println(err)
	}

	c.Pid = cmd.Process.Pid
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

	if isNginx {
		bridgeip := net.ParseIP(s.BridgeIP)
		c.IP = net.IPv4(bridgeip[12], bridgeip[13], bridgeip[14], 2).String()
	} else {
		c.IP = s.NextIP()
	}

	if err := c.Exec(fmt.Sprintf("/sbin/ifconfig veth1 %s", c.IP)); err != nil {
		fmt.Println(err)
		return
	}

	containers[c.Name] = c
	s.Containers[c.Name] = c

	if isNginx {
		s.Pid = c.Pid
	} else {
		s.Servers = append(s.Servers, fmt.Sprintf("%s:8080", c.IP))
		s.WriteConfig(CONTAIN_DIR)
		s.Reload()
	}
	services[c.ServiceName] = s

	fmt.Println(cmd.Process.Pid)
	c.WriteConfig()

	cmd.Wait()

	c.Active = false
	s.ReturnIP(c.IP)
	delete(containers, c.Name)
	delete(services[c.ServiceName].Containers, c.Name)
}
