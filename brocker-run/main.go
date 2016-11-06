package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

var wg sync.WaitGroup

func main() {
	fmt.Println("running child")
	args := []string{os.Args[1]}
	args = append(args, os.Args[2:]...)
	fmt.Println(args)
	syscall.Exec(os.Args[1], args, os.Environ())
}
