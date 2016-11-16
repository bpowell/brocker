package container

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Contains all information needed to run a container.
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

// SetName sets the name of the container based on start time and the command being run.
func (c *Container) SetName() {
	value := fmt.Sprintf("%s%s", c.StartTime, c.Command)
	sha := sha1.New()
	sha.Write([]byte(value))
	c.Name = hex.EncodeToString(sha.Sum(nil))[:8]
}

// Close shutdowns the container.
func (c *Container) Close() {
	if err := c.Exec("/bin/umount /app"); err != nil {
		fmt.Println("Cannot unmount /app: ", err)
	}

	p, _ := os.FindProcess(c.Pid)
	p.Kill()
}

// Exec executes command inside of the container.
func (c *Container) Exec(cmd string) error {
	command := strings.Split(fmt.Sprintf("nsenter --target %d --pid --net --mount %s", c.Pid, cmd), " ")
	if err := exec.Command(command[0], command[1:]...).Run(); err != nil {
		return err
	}

	return nil
}
