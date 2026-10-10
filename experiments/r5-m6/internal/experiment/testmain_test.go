package experiment

import (
	"fmt"
	"io"
	"net"
	"os"
	"testing"
)

func TestMain(suite *testing.M) {
	listeners := []net.Listener{}
	if os.Getenv("R5_TEST_NETWORK") == "compose" {
		if os.Getenv("R5_INTEGRATION") != "1" {
			fmt.Fprintln(os.Stderr, "compose proxy only supports isolated integration")
			os.Exit(1)
		}
		for _, endpoint := range []struct{ local, remote string }{{"127.0.0.1:23306", "gm-r5-verify-mysql-1:23306"}, {"127.0.0.1:25672", "gm-r5-verify-rabbitmq-1:5672"}} {
			listener, err := net.Listen("tcp", endpoint.local)
			if err != nil {
				fmt.Fprintln(os.Stderr, "isolated race listener unavailable")
				os.Exit(1)
			}
			listeners = append(listeners, listener)
			go proxyTestConnections(listener, endpoint.remote)
		}
	}
	code := suite.Run()
	for _, listener := range listeners {
		listener.Close()
	}
	os.Exit(code)
}
func proxyTestConnections(listener net.Listener, target string) {
	for {
		incoming, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer incoming.Close()
			outgoing, err := net.DialTimeout("tcp", target, 3e9)
			if err != nil {
				return
			}
			defer outgoing.Close()
			done := make(chan struct{})
			go func() { io.Copy(incoming, outgoing); incoming.Close(); close(done) }()
			io.Copy(outgoing, incoming)
			outgoing.Close()
			<-done
		}()
	}
}
