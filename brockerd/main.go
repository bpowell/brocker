package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"text/template"
	"time"
)

type Service struct {
	ContainterName string
	Name           string `json:"name"`
	BridgeName     string
	BridgeIP       string `json:"bridge-ip"`
	Pid            int
	Containers     map[string]Container
	NginxUpStream
}

type Container struct {
	Name        string
	ServiceName string `json:"service-name"`
	Command     string `json:"command"`
	CopyFile    bool   `json:"copy-file"`
	FileToCopy  string `json:"file"`
	Pid         int
	IP          string
	StartTime   time.Time
	VEth        string
}

type NginxUpStream struct {
	LoadBalanceType string
	Servers         []string
}

var services map[string]Service
var containers map[string]Container
var nginxConfig *template.Template

const (
	bridgeNameBase = "brocker"
	vethNameBase   = "veth"
	MOUNT_LOC      = "/app"
	CONTAIN_DIR    = "/container"
)

func (c *Container) setName() {
	value := fmt.Sprintf("%s%s", c.StartTime, c.Command)
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
	if err := execInContainter("/usr/sbin/nginx -s reload -c /app/nginx.conf", s.Pid); err != nil {
		fmt.Println("Cannot reload nginx: ", err)
		return
	}
}

func (s *Service) Stop() {
	if err := execInContainter("/usr/sbin/nginx -s stop -c /app/nginx.conf", s.Pid); err != nil {
		fmt.Println(err)
	}

	for _, c := range s.Containers {
		c.Close()
	}

	deleteBridge := strings.Split(fmt.Sprintf("ip link delete %s type bridge", s.BridgeName), " ")
	if err := exec.Command(deleteBridge[0], deleteBridge[1:]...).Run(); err != nil {
		fmt.Printf("Cannot delete bridge %s", s.BridgeName)
	}
}

func (s *Service) writeConfig() {
	myappconffile, err := os.OpenFile(fmt.Sprintf("%s/%s/myapp.conf", CONTAIN_DIR, s.ContainterName), os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer myappconffile.Close()

	if err := nginxConfig.ExecuteTemplate(myappconffile, "myapp.conf.tmpl", s); err != nil {
		fmt.Println(err)
		return
	}
}

func init() {
	services = make(map[string]Service)
	containers = make(map[string]Container)
	nginxConfig = template.Must(template.ParseFiles("/etc/brocker/nginx.conf.tmpl", "/etc/brocker/myapp.conf.tmpl"))
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
	http.HandleFunc("/api/v1/container/rm", containerRm)
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

	s.BridgeName = fmt.Sprintf("%s%d", bridgeNameBase, len(services)+1)

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

	c := Container{
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

func containerList(w http.ResponseWriter, r *http.Request) {
	if err := json.NewEncoder(w).Encode(containers); err != nil {
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

func containerRm(w http.ResponseWriter, r *http.Request) {
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

func serviceCreateNetwork(s Service) error {
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

func run(c Container, isNginx bool) {
	fmt.Println("running parent")
	s := services[c.ServiceName]
	runcmd, err := exec.LookPath("brocker-run")
	if err != nil {
		fmt.Println(err)
		return
	}

	c.StartTime = time.Now()
	c.setName()

	if err := os.Mkdir(fmt.Sprintf("%s/%s", CONTAIN_DIR, c.Name), 0644); err != nil {
		fmt.Println(err)
		return
	}

	if c.CopyFile {
		if err := exec.Command("cp", c.FileToCopy, fmt.Sprintf("%s/%s/%s", CONTAIN_DIR, c.Name, path.Base(c.FileToCopy))).Run(); err != nil {
			fmt.Println(err)
			return
		}
	}

	if isNginx {
		nginxconffile, err := os.Create(fmt.Sprintf("%s/%s/nginx.conf", CONTAIN_DIR, c.Name))
		if err != nil {
			fmt.Println(err)
			nginxconffile.Close()
			return
		}

		if err := nginxConfig.ExecuteTemplate(nginxconffile, "nginx.conf.tmpl", s); err != nil {
			fmt.Println(err)
			nginxconffile.Close()
			return
		}
		nginxconffile.Close()

		myappconffile, err := os.Create(fmt.Sprintf("%s/%s/myapp.conf", CONTAIN_DIR, c.Name))
		if err != nil {
			fmt.Println(err)
			myappconffile.Close()
			return
		}

		if err := nginxConfig.ExecuteTemplate(myappconffile, "myapp.conf.tmpl", s); err != nil {
			fmt.Println(err)
			myappconffile.Close()
			return
		}
		myappconffile.Close()
	}

	args := strings.Split(fmt.Sprintf("%s %s %s", runcmd, c.Name, c.Command), " ")

	cmd := &exec.Cmd{
		Path: runcmd,
		Args: args,
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

	containers[c.Name] = c
	s.Containers[c.Name] = c

	if isNginx {
		s.Pid = c.Pid
		s.ContainterName = c.Name
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
