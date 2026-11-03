//go:build integration

package integration_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
)

// silentProxy forwards connections to a server. After silence, the connections
// it holds stay open but carry nothing in either direction, as when packets to
// a host start to vanish, while new connections work.
type silentProxy struct {
	listener net.Listener
	target   string
	era      atomic.Int64
}

func startSilentProxy(t *testing.T, target string) *silentProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &silentProxy{listener: listener, target: target}
	t.Cleanup(func() { _ = listener.Close() })
	go proxy.serve()
	return proxy
}

func (p *silentProxy) serve() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", p.target)
		if err != nil {
			_ = client.Close()
			continue
		}
		era := p.era.Load()
		go p.forward(client, server, era)
		go p.forward(server, client, era)
	}
}

// forward copies from one side to the other until the era ends, and from then
// on reads and discards, so that nothing shows the other side a closed socket.
func (p *silentProxy) forward(from, to net.Conn, era int64) {
	defer from.Close()
	defer to.Close()
	buffer := make([]byte, 32<<10)
	for {
		count, err := from.Read(buffer)
		if count > 0 && p.era.Load() == era {
			if _, err := to.Write(buffer[:count]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// testSilentConnection checks that calls recover from a connection that goes
// silent: calls fail at their deadlines until keepalive closes the connection,
// and the next call reconnects. No write that reached no server takes effect.
func (s *system) testSilentConnection(t *testing.T) {
	t.Helper()
	backend := s.backends[0]
	proxy := startSilentProxy(t, s.cluster.Address())
	client, err := weir.New(proxy.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	put := func(n int64, timeout time.Duration) (*weir.MutationResult, error) {
		request := backend.write(t, "silent", n)
		options := weir.WriteOptions{StoreName: backend.name, Request: request}
		rpc, cancel := context.WithTimeout(s.ctx, timeout)
		defer cancel()
		return client.Put(rpc, options)
	}
	result, err := put(1, rpcTimeout)
	assertApplied(t, result, err)
	proxy.era.Add(1)
	started := time.Now()
	failed := 0
	var last int64
	for n := int64(2); last == 0; n++ {
		result, err := put(n, time.Second)
		if err == nil && result.GetOutcome() == weir.MutationApplied {
			last = n
			break
		}
		failed++
		if elapsed := time.Since(started); elapsed > 25*time.Second {
			t.Fatalf("calls did not recover from a silent connection within %s: %v %v", elapsed, result, err)
		}
	}
	if failed == 0 {
		t.Fatal("no call noticed the silent connection")
	}
	t.Logf("recovered after %s and %d failed calls", time.Since(started), failed)
	want := record{id: "silent", n: last}
	backend.assertRead(t, s.ctx, client, want)
}
