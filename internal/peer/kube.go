package peer

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ncmink/biebie-protocol/deeplink"
	"github.com/ncmink/biebie-protocol/ipc"
)

const kubeOpenTimeout = 2 * time.Second

// DeliverKubeLink asks a running Biebie Kube to open a handoff over IPC.
//
// false means nothing answered, which is ordinary when Biebie Kube is not
// running yet and the caller should fall back to a deep link.
func DeliverKubeLink(link string) bool {
	if _, err := deeplink.ParseOpen(link); err != nil {
		return false
	}
	endpoint, err := ipc.KubeEndpoint()
	if err != nil {
		return false
	}
	client := ipc.NewClient(endpoint).WithTimeout(kubeOpenTimeout)
	err = client.Call(context.Background(), ipc.Method("app.open"), map[string]string{"link": link}, nil)
	return err == nil
}

// LaunchKubeLink opens Biebie Kube through the desktop's URL handler.
func LaunchKubeLink(link string) error {
	if _, err := deeplink.ParseOpen(link); err != nil {
		return err
	}
	return openKubeURL(link)
}

func openKubeURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return runKubeHandler(exec.Command("/usr/bin/open", url))
	case "windows":
		return runKubeHandler(exec.Command("rundll32", "url.dll,FileProtocolHandler", url))
	default:
		return runKubeHandler(exec.Command("xdg-open", url))
	}
}

func runKubeHandler(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil {
			return nil
		}
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return errors.New(detail)
		}
		return err
	case <-time.After(kubeOpenTimeout):
		return nil
	}
}
