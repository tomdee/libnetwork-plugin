package netns

import (
	"errors"
	"os/exec"
	"regexp"
	"time"
)

const (
	IPCmdTimeout = 5
)

var (
	IPv6RE = regexp.MustCompile(`inet6 ([a-fA-F\d:]+)/\d{1,3}`)
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

func BringUpInterface(interfaceName string) error {
	cmd := exec.Command("ip", "link", "set", interfaceName, "up")
	return ExecuteWithTimeout(cmd, IPCmdTimeout)
}

func GetIPv6LinkLocal(interfaceName string) ([]byte, error) {
	var (
		out []byte
		err error
		cmd = exec.Command("ip", "-6", "addr", "show", "dev", interfaceName)
	)
	if out, err = cmd.Output(); err != nil {
		return nil, err
	}
	matches := IPv6RE.FindAll(out, -1)
	if len(matches) > 1 {
		return matches[1], nil
	}
	return nil, errors.New("IP not found")
}
