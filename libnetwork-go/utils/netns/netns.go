package netns

import (
	"os/exec"
	"time"
)

const (
	IPCmdTimeout = 5
)

func ExecuteWithTimeout(cmd *exec.Cmd, seconds int) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	timer := time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		cmd.Process.Kill()
	})
	err := cmd.Wait()
	timer.Stop()
	if err != nil {
		return err
	}
	return nil
}

func CreateVeth(vethNameHost, vethNameNSTemp string) error {
	cmd := exec.Command(
		"ip", "link", "add", vethNameHost,
		"type", "veth", "peer", "name", vethNameNSTemp)
	if err := ExecuteWithTimeout(cmd, IPCmdTimeout); err != nil {
		return err
	}

	cmd = exec.Command("ip", "link", "set", vethNameHost, "up")
	if err := ExecuteWithTimeout(cmd, IPCmdTimeout); err != nil {
		return err
	}
	return nil
}

func SetVethMac(vethNameHost, mac string) error {
	cmd := exec.Command("ip", "link", "set", "dev", vethNameHost, "address", mac)
	if err := ExecuteWithTimeout(cmd, IPCmdTimeout); err != nil {
		return err
	}
	return nil
}

func RemoveVeth(vethNameHost string) (bool, error) {
	if !IsVethExists(vethNameHost) {
		return false, nil
	}
	cmd := exec.Command("ip", "link", "del", vethNameHost)
	if err := ExecuteWithTimeout(cmd, IPCmdTimeout); err != nil {
		return false, err
	}
	return true, nil
}

func IsVethExists(vethHostName string) bool {
	cmd := exec.Command("ip", "link", "show", vethHostName)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return true
}
