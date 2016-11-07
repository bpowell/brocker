package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

const (
	ADD_SERVICE    = "http://localhost:3000/api/v1/service/add"
	RUN_CONTAINER  = "http://localhost:3000/api/v1/container/run"
	LIST_CONTAINER = "http://localhost:3000/api/v1/container/list"
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

func list_containers() {
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

func help() {
}

func main() {
	switch os.Args[1] {
	case "service":
		switch os.Args[2] {
		case "add":
			call(ADD_SERVICE)
		}
	case "container":
		switch os.Args[2] {
		case "run":
			call(RUN_CONTAINER)
		case "list":
			list_containers()
		}
	}
}
