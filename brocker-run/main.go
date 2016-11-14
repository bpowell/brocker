package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func main() {
	fmt.Println("running child")
	/*
		Allows the use of CLONE_NEWNS on ubuntu boxes. util-linux <= 2.27 have issues
		with systemd making / shared across all namespaces
	*/
	remount := strings.Split("/bin/mount --make-private -o remount /", " ")
	if err := exec.Command(remount[0], remount[1:]...).Run(); err != nil {
		fmt.Println(err)
	}

	mount_app := strings.Split(fmt.Sprintf("/bin/mount --bind /container/%s /app", os.Args[1]), " ")
	if err := exec.Command(mount_app[0], mount_app[1:]...).Run(); err != nil {
		fmt.Println(err)
	}

	fmt.Println(syscall.Exec(os.Args[2], os.Args[2:], os.Environ()))

	if err := syscall.Unmount("/app", 0); err != nil {
		fmt.Println("Cannot unmount /app", err)
	}
}
