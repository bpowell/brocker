package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Service struct {
	Name       string `json:"name"`
	BridgeName string
	BridgeIP   string `json:"bridge-ip"`
	NginxConf  string `json:"nginx-config"`
	Pid        int
	Containers map[string]Container
	NginxUpStream
}

type Container struct {
	Name        string
	ServiceName string `json:"service-name"`
	Command     string `json:"command"`
	Pid         int
	IP          string
	StartTime   time.Time
	VEth        string
}

type NginxUpStream struct {
	LoadBalanceType string
	Servers         []string
	UpStreamConfig  string `json:"nginx-upstream"`
}

var services map[string]Service
var containers map[string]Container

const (
	bridgeNameBase = "brocker"
	vethNameBase   = "veth"
	MOUNT_LOC      = "/app"
	CONTAIN_DIR    = "/container"
)

func (c *Container) setName() {
	value := fmt.Sprintf("%s%s%s", c.Name, c.StartTime, c.Command)
	sha := sha1.New()
	sha.Write([]byte(value))
	c.Name = hex.EncodeToString(sha.Sum(nil))[:8]
}

func (c *Container) Close() {
	if err := execInContainter("/bin/umount /app", c.Pid); err != nil {
		fmt.Println("Cannot unmount /app: ", err)
	}

	p, _ := os.FindProcess(c.Pid)
	p.Kill()
}

func (s *Service) reload() {
	if err := execInContainter(fmt.Sprintf("/usr/sbin/nginx -s reload -c %s", s.NginxConf), s.Pid); err != nil {
		fmt.Println("Cannot reload nginx: ", err)
		return
	}
}

func (s *Service) Stop() {
	if err := execInContainter(fmt.Sprintf("/usr/sbin/nginx -s stop -c %s", s.NginxConf), s.Pid); err != nil {
		fmt.Println(err)
	}

	for _, c := range s.Containers {
		c.Close()
	}

	delete_bridge := strings.Split(fmt.Sprintf("ip link delete %s type bridge", s.BridgeName), " ")
	if err := exec.Command(delete_bridge[0], delete_bridge[1:]...).Run(); err != nil {
		fmt.Printf("Cannot delete bridge %s", s.BridgeName)
	}
}

func (n *NginxUpStream) writeConfig() {
	if _, err := os.Stat(n.UpStreamConfig); os.IsNotExist(err) {
		fmt.Println("Cannot update config", err)
		return
	}

	var buffer bytes.Buffer
	buffer.WriteString("upstream myapp1 {\n")
	buffer.WriteString(n.LoadBalanceType)
	buffer.WriteString(";\n")
	for _, s := range n.Servers {
		buffer.WriteString(fmt.Sprintf("server %s;\n", s))
	}
	buffer.WriteString("\n}")

	if err := ioutil.WriteFile(n.UpStreamConfig, buffer.Bytes(), 0644); err != nil {
		fmt.Println(err)
		return
	}
}

func init() {
	services = make(map[string]Service)
	containers = make(map[string]Container)
}

func main() {
	ctrl_c := make(chan os.Signal, 1)
	signal.Notify(ctrl_c, os.Interrupt)
	go func() {
		for _ = range ctrl_c {
			for _, s := range services {
				s.Stop()
			}
			os.Exit(0)
		}
	}()

	http.HandleFunc("/api/v1/service/add", service_add)
	http.HandleFunc("/api/v1/container/run", container_run)
	http.HandleFunc("/api/v1/container/list", container_list)
	http.HandleFunc("/api/v1/container/exec", container_exec)
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

	s := Service{
		Containers: make(map[string]Container),
	}

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

	s.LoadBalanceType = "least_conn"
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

	go run(c, true)

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

	go run(c, false)

	w.WriteHeader(http.StatusCreated)
}

func container_list(w http.ResponseWriter, r *http.Request) {
	if err := json.NewEncoder(w).Encode(containers); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func container_exec(w http.ResponseWriter, r *http.Request) {
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

func run(c Container, isNginx bool) {
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
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET,
	}

	if err := cmd.Start(); err != nil {
		fmt.Println(err)
	}

	c.StartTime = time.Now()
	c.setName()

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

	/*
		Allows the use of CLONE_NEWNS on ubuntu boxes. util-linux <= 2.27 have issues
		with systemd making / shared across all namespaces
	*/
	if err := execInContainter("/bin/mount --make-private -o remount /", c.Pid); err != nil {
		fmt.Println(err)
		return
	}

	if err := os.Mkdir(fmt.Sprintf("%s/%s", CONTAIN_DIR, c.Name), 0644); err != nil {
		fmt.Println(err)
		return
	}

	if err := execInContainter(fmt.Sprintf("/bin/mount --bind %s/%s %s", CONTAIN_DIR, c.Name, MOUNT_LOC), c.Pid); err != nil {
		fmt.Println(err)
		return
	}

	containers[c.Name] = c
	s.Containers[c.Name] = c

	if isNginx {
		s.Pid = c.Pid
	} else {
		s.Servers = append(s.Servers, fmt.Sprintf("%s:8080", c.IP))
		s.writeConfig()
		s.reload()
	}
	services[c.ServiceName] = s

	fmt.Println(cmd.Process.Pid)

	cmd.Wait()

	delete(containers, c.Name)
	delete(services[c.ServiceName].Containers, c.Name)
}

func execInContainter(cmd string, pid int) error {
	command := strings.Split(fmt.Sprintf("nsenter --target %d --pid --net --mount %s", pid, cmd), " ")
	if err := exec.Command(command[0], command[1:]...).Run(); err != nil {
		return err
	}

	return nil
}
