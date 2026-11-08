package TCP

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func readMySQLPacket(t *testing.T, r io.Reader) (byte, []byte) {
	t.Helper()
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		t.Fatalf("read packet header: %v", err)
	}
	length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("read %d byte payload: %v", length, err)
	}
	return header[3], payload
}

func TestMySQLService_GreetingAndDeniedLogin(t *testing.T) {
	addr, tr := vncStartService(t, "../../../../configurations/services/tcp-3306.yaml")

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	seq, greeting := readMySQLPacket(t, conn)
	if seq != 0 || greeting[0] != 0x0a {
		t.Fatalf("greeting seq=%d protocol=%#x, want 0 and 0x0a", seq, greeting[0])
	}
	if !bytes.HasPrefix(greeting[1:], []byte("8.0.29\x00")) {
		t.Fatalf("greeting version = %q", greeting[1:9])
	}
	if !bytes.Contains(greeting, []byte("caching_sha2_password\x00")) {
		t.Fatalf("greeting has no auth plugin: %q", greeting)
	}

	// HandshakeResponse41 for user "backup_admin".
	login := make([]byte, 32)
	binary.LittleEndian.PutUint32(login, 0x000fa685)
	login = append(login, "backup_admin\x00\x00"...)
	packet := append([]byte{byte(len(login)), 0, 0, 1}, login...)
	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("write login: %v", err)
	}

	seq, denied := readMySQLPacket(t, conn)
	if seq != 2 || denied[0] != 0xff {
		t.Fatalf("reply seq=%d first=%#x, want 2 and 0xff", seq, denied[0])
	}
	if code := binary.LittleEndian.Uint16(denied[1:3]); code != 1045 {
		t.Fatalf("error code = %d, want 1045", code)
	}
	if bytes.Contains(denied, []byte("root")) {
		t.Fatalf("denial names a user the client did not send: %q", denied)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range tr.snapshot() {
			if ev.Handler == "login_denied" && bytes.Contains([]byte(ev.Command), []byte("backup_admin")) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("login username was not recorded in a trace event")
}
