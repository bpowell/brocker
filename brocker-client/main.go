package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func add_service(filename string) {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println(err)
		return
	}

	req, err := http.NewRequest("POST", "http://localhost:3000/api/v1/service/add", bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		fmt.Println("Cannot add service")
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println(string(body))
	}
}

func help() {
}

func main() {
	switch os.Args[1] {
	case "service":
		switch os.Args[2] {
		case "add":
			if len(os.Args) != 4 {
				help()
				return
			}
			add_service(os.Args[3])
		}
	}
}
