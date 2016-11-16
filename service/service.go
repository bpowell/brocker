package service

import (
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"strings"

	"github.com/bpowell/brocker/container"
)

var nginxConfig *template.Template

func init() {
	nginxConfig = template.Must(template.ParseFiles("/etc/brocker/nginx.conf.tmpl", "/etc/brocker/myapp.conf.tmpl"))
}

type Service struct {
	ContainterName  string
	Name            string `json:"name"`
	BridgeName      string
	BridgeIP        string `json:"bridge-ip"`
	Pid             int
	Containers      map[string]container.Container
	LoadBalanceType string
	Servers         []string
}

func (s *Service) Reload() {
	c, ok := s.Containers[s.ContainterName]
	if !ok {
		fmt.Println("Not a container", s.ContainterName)
		return
	}

	if err := c.Exec("/usr/sbin/nginx -s reload -c /app/nginx.conf"); err != nil {
		fmt.Println("Cannot reload nginx: ", err)
		return
	}
}

func (s *Service) Stop() {
	c, ok := s.Containers[s.ContainterName]
	if !ok {
		fmt.Println("Not a container", s.ContainterName)
		return
	}

	if err := c.Exec("/usr/sbin/nginx -s stop -c /app/nginx.conf"); err != nil {
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

func (s *Service) WriteConfig(path string) {
	nginxconffile, err := os.Create(fmt.Sprintf("%s/%s/nginx.conf", path, s.ContainterName))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer nginxconffile.Close()

	if err := nginxConfig.ExecuteTemplate(nginxconffile, "nginx.conf.tmpl", s); err != nil {
		fmt.Println(err)
		nginxconffile.Close()
		return
	}

	myappconffile, err := os.Create(fmt.Sprintf("%s/%s/myapp.conf", path, s.ContainterName))
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
