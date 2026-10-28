//go:build darwin || linux

package peer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ncmink/biebie-protocol/deeplink"
	"github.com/ncmink/biebie-protocol/ipc"
)

func TestKubeOpenLinkIsDeliveredOverIPC(t *testing.T) {
	dir, err := os.MkdirTemp(".", "sock")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	endpoint := ipc.Endpoint{Dir: dir, Address: filepath.Join(dir, "kube.sock")}
	var mu sync.Mutex
	var received string

	srv := ipc.NewServer(endpoint)
	srv.Handle(ipc.Method("app.open"), func(_ context.Context, params json.RawMessage) (any, error) {
		var req struct {
			Link string `json:"link"`
		}
		_ = json.Unmarshal(params, &req)
		mu.Lock()
		received = req.Link
		mu.Unlock()
		return map[string]bool{"accepted": true}, nil
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("start kube: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	link, err := deeplink.OpenKube("hnd_ctctctctctctctct")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := ipc.NewClient(endpoint).Call(context.Background(), ipc.Method("app.open"), map[string]string{"link": link}, nil); err != nil {
		t.Fatalf("call: %v", err)
	}

	mu.Lock()
	got := received
	mu.Unlock()
	if got != link {
		t.Fatalf("received = %q, want %q", got, link)
	}
}
