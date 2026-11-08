package TCP

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// respRequest encodes args the way redis-cli and client libraries send them.
func respRequest(args ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return sb.String()
}

// readRESPReply reads one complete reply, including every element of an array
// or map, and fails if a declared bulk length does not line up with the bytes
// that follow it. Aggregates return their header line.
func readRESPReply(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read reply line: %v", err)
	}
	line = strings.TrimRight(line, "\r\n")
	switch line[0] {
	case '$':
		size, err := strconv.Atoi(line[1:])
		if err != nil {
			t.Fatalf("bulk length in %q: %v", line, err)
		}
		body := make([]byte, size+2)
		if _, err := io.ReadFull(r, body); err != nil {
			t.Fatalf("read %d byte bulk reply: %v", size, err)
		}
		if string(body[size:]) != "\r\n" {
			t.Fatalf("bulk reply not terminated after %d bytes: %q", size, body[size:])
		}
		return string(body[:size])
	case '*', '%':
		count, err := strconv.Atoi(line[1:])
		if err != nil {
			t.Fatalf("aggregate length in %q: %v", line, err)
		}
		if line[0] == '%' {
			count *= 2
		}
		for range count {
			readRESPReply(t, r)
		}
	}
	return line
}

// redisExchange sends one request to addr on a new connection and returns the reply.
func redisExchange(t *testing.T, addr, request string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write: %v", err)
	}
	return readRESPReply(t, bufio.NewReader(conn))
}

func TestRedisService_RealClientRequests(t *testing.T) {
	// vncStartService loads any shipped TCP service file.
	addr, _ := vncStartService(t, "../../../../configurations/services/tcp-6379-redis.yaml")

	cases := []struct {
		name, request, want string
	}{
		{"resp ping", respRequest("PING"), "+PONG"},
		{"resp lowercase ping", respRequest("ping"), "+PONG"},
		{"inline ping", "PING\r\n", "+PONG"},
		{"resp hello 3", respRequest("HELLO", "3"), "%7"},
		{"resp auth", respRequest("AUTH", "secret"), "-ERR AUTH <password> called without any password configured for the default user. Are you sure your configuration is correct?"},
		{"resp info", respRequest("INFO", "server"), infoReply},
		{"resp unknown", respRequest("MODULE", "LIST"), "-ERR unknown command"},
		{"longer command name is unknown", respRequest("PINGPONG"), "-ERR unknown command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redisExchange(t, addr, tc.request); got != tc.want {
				t.Fatalf("reply = %q, want %q", got, tc.want)
			}
		})
	}
}

const infoReply = "# Server\r\nredis_version:7.0.12\r\nos:Linux x86_64\r\ntcp_port:6379\r\nuptime_in_seconds:0\r\nuptime_in_days:0\r\nconnected_clients:1\r\n"
