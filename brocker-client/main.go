package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	ADD_SERVICE    = "http://localhost:3000/api/v1/service/add"
	RUN_CONTAINER  = "http://localhost:3000/api/v1/container/run"
	LIST_CONTAINER = "http://localhost:3000/api/v1/container/list"
	EXEC_CONTAINER = "http://localhost:3000/api/v1/container/exec"
	STOP_CONTAINER = "http://localhost:3000/api/v1/container/stop"
)

func call(url string) {
	if len(os.Args) != 4 {
		help()
		return
	}

	filename := os.Args[3]

	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println(err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println(string(body))
	}
}

func listContainers() {
	resp, err := http.Get(LIST_CONTAINER)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(body))
}

func execContainer() {
	if len(os.Args) <= 4 {
		help()
		return
	}

	raw := []byte(fmt.Sprintf("{\"name\":\"%s\"}", os.Args[3]))
	fmt.Println(string(raw))
	req, err := http.NewRequest("POST", EXEC_CONTAINER, bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	pid := string(body)
	cmd := strings.Join(os.Args[4:], " ")
	path, err := exec.LookPath("nsenter")
	if err != nil {
		fmt.Println("Cannot find nsenter")
		return
	}

	command := strings.Split(fmt.Sprintf("%s --target %s --pid --net --mount %s", path, pid, cmd), " ")
	run := &exec.Cmd{
		Path: command[0],
		Args: command,
	}

	run.Stdin = os.Stdin
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	run.Start()
	run.Wait()
}

func stopContainer() {
	if len(os.Args) != 4 {
		help()
		return
	}

	raw := []byte(fmt.Sprintf("{\"name\":\"%s\"}", os.Args[3]))
	fmt.Println(string(raw))
	req, err := http.NewRequest("POST", STOP_CONTAINER, bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(body))
}

func help() {
	fmt.Println("Commands:")
	fmt.Println("\tcontainer")
	fmt.Println("\tservice")
	fmt.Println()

	fmt.Println("container:")
	fmt.Println("\trun filename.json - Runs command specified in filename.json")
	fmt.Println("\texec container_hash command... - Runs specified command in container")
	fmt.Println("\tlist - Lists all running containers")
	fmt.Println("\tstop container_hash - Stops container")
	fmt.Println()

	fmt.Println("service:")
	fmt.Println("\tadd filename.json - Creates a service with details from filename.json")
}

func main() {
	if len(os.Args) < 3 {
		help()
		return
	}

	switch os.Args[1] {
	case "service":
		switch os.Args[2] {
		case "add":
			call(ADD_SERVICE)
		default:
			help()
		}
	case "container":
		switch os.Args[2] {
		case "run":
			call(RUN_CONTAINER)
		case "list":
			listContainers()
		case "exec":
			execContainer()
		case "stop":
			stopContainer()
		default:
			help()
		}
	default:
		help()
	}
}
