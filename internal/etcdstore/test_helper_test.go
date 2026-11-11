package etcdstore_test

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// StartEmbeddedEtcd starts an in-process etcd cluster node for hermetic testing.
func StartEmbeddedEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	t.Helper()

	dir := filepath.Join(os.TempDir(), fmt.Sprintf("etcd-test-%d", time.Now().UnixNano()))

	cfg := embed.NewConfig()
	cfg.Dir = dir
	cfg.LogLevel = "error"

	// Pick random loopback ports
	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")

	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("failed to start embedded etcd: %v", err)
	}

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Close()
		_ = os.RemoveAll(dir)
		t.Fatal("embedded etcd took too long to start")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		_ = os.RemoveAll(dir)
		t.Fatalf("failed to create etcd client: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		e.Close()
		_ = os.RemoveAll(dir)
	}

	return e, client, cleanup
}
