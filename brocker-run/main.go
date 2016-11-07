package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
)

var wg sync.WaitGroup

func main() {
	fmt.Println("running child")
	args := strings.Split(os.Args[1], " ")
	args = append(args, os.Args[2:]...)
	fmt.Println(syscall.Exec(args[0], args, os.Environ()))
}
