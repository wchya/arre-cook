package mailer

import (
	"context"
	"net"
	"ninimenu/internal/config"
	"strconv"
	"testing"
	"time"
)

func TestSMTPGreetingStallRespectsCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	old := config.C
	t.Cleanup(func() { config.C = old })
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	config.C = config.Config{SMTPHost: host, SMTPPort: n, SMTPUser: "test@example.test", SMTPPassword: "fixture"}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		var b [1]byte
		_, _ = conn.Read(b[:])
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := Send("test@example.test", "fixture", "fixture", ctx); err == nil {
		t.Fatal("stalled SMTP succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("SMTP ignored deadline")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close socket")
	}
}
