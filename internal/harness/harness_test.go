package harness

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func request(t *testing.T, client *http.Client, method, url, token string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, url, r)
	if e != nil {
		t.Fatal(e)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {
		t.Fatal(e)
	}
	return resp.StatusCode, b
}
func expect(t *testing.T, actual, want int, body []byte) {
	t.Helper()
	if actual != want {
		t.Fatalf("HTTP %d want %d: %s", actual, want, body)
	}
}
func payload(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var d map[string]any
	if e := json.Unmarshal(b, &d); e != nil {
		t.Fatal(e)
	}
	return d
}
func TestEndToEnd(t *testing.T) {
	root := t.TempDir()
	copyAssets(t, root)
	a, e := New(root, "", "")
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	url := srv.URL
	client := srv.Client()
	token := a.TokenHex()
	status, b := request(t, client, "GET", url+"/health", "", nil)
	expect(t, status, 200, b)
	status, b = request(t, client, "GET", url+"/api/state", "", nil)
	expect(t, status, 401, b)
	status, b = request(t, client, "GET", url+"/", "", nil)
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte("LIVE OPERATIONS")) {
		t.Fatal("live UI missing")
	}
	status, b = request(t, client, "GET", url+"/live.js", "", nil)
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte("/api/execute")) {
		t.Fatal("API wiring absent")
	}
	status, b = request(t, client, "POST", url+"/api/login", "", map[string]string{"token": "bad"})
	expect(t, status, 403, b) // curl without Origin or bearer always blocked
	status, b = request(t, client, "POST", url+"/api/login", token, map[string]string{"token": token})
	expect(t, status, 200, b)
	q := Request{Tool: "write", Path: "notes/demo.txt", Content: "live data"}
	status, b = request(t, client, "POST", url+"/api/evaluate", token, q)
	expect(t, status, 200, b)
	if payload(t, b)["decision"] != "ALLOW" {
		t.Fatalf("eval %s", b)
	}
	status, b = request(t, client, "POST", url+"/api/execute", token, q)
	expect(t, status, 200, b)
	id := payload(t, b)["id"].(string)
	file, e := os.ReadFile(filepath.Join(root, "data", "workspace", "notes", "demo.txt"))
	if e != nil || string(file) != "live data" {
		t.Fatalf("workspace not written: %s %v", file, e)
	}
	status, b = request(t, client, "POST", url+"/api/execute", token, Request{Tool: "read", Path: "notes/demo.txt"})
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte("live data")) {
		t.Fatal("read failed")
	}
	status, b = request(t, client, "POST", url+"/api/execute", token, Request{Tool: "hash", Path: "notes/demo.txt"})
	expect(t, status, 200, b)
	status, b = request(t, client, "POST", url+"/api/execute", token, Request{Tool: "list", Path: "notes"})
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte("demo.txt")) {
		t.Fatal("list failed")
	}
	status, b = request(t, client, "POST", url+"/api/execute", token, Request{Tool: "shell", Path: "rm -rf /"})
	expect(t, status, 403, b)
	if !bytes.Contains(b, []byte("DANGEROUS_COMMAND")) {
		t.Fatal("command not blocked")
	}
	for _, path := range []string{"../out.txt", "/tmp/out.txt", "notes/../../out.txt", "notes//out.txt", "notes/./out.txt"} {
		status, b = request(t, client, "POST", url+"/api/execute", token, Request{Tool: "write", Path: path, Content: "attack"})
		expect(t, status, 403, b)
	}
	if e := os.Symlink(t.TempDir(), filepath.Join(root, "data", "workspace", "evil")); e != nil {
		t.Fatal(e)
	}
	status, b = request(t, client, "POST", url+"/api/execute", token, Request{Tool: "write", Path: "evil/escape.txt", Content: "attack"})
	expect(t, status, 422, b)
	status, b = request(t, client, "GET", url+"/api/verify", token, nil)
	expect(t, status, 200, b)
	if payload(t, b)["verified"] != true {
		t.Fatal("verification unsuccessful")
	}
	status, b = request(t, client, "GET", url+"/api/replay/"+id, token, nil)
	expect(t, status, 200, b)
	if payload(t, b)["matches_recorded_after"] != true {
		t.Fatalf("replay did not match: %s", b)
	}
	status, b = request(t, client, "GET", url+"/api/export/sft", token, nil)
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte(`"source_event"`)) {
		t.Fatal("SFT row absent")
	}
	status, b = request(t, client, "GET", url+"/api/export/preferences", token, nil)
	expect(t, status, 200, b)
	if len(b) != 0 {
		t.Fatal("invented preference rows")
	}
	status, b = request(t, client, "POST", url+"/api/feedback", token, map[string]string{"prompt": "How?", "chosen": "safe", "rejected": "unsafe"})
	expect(t, status, 201, b)
	status, b = request(t, client, "GET", url+"/api/export/preferences", token, nil)
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte(`"chosen":"safe"`)) {
		t.Fatal("feedback not exported")
	}
	status, b = request(t, client, "GET", url+"/api/state", token, nil)
	expect(t, status, 200, b)
	if payload(t, b)["denied"].(float64) < 6 {
		t.Fatal("blocked attempts not counted")
	}
	status, b = request(t, client, "POST", url+"/api/infer", token, map[string]string{"prompt": "hello"})
	expect(t, status, 503, b)
	// Restart verifies the persisted hash/MAC chain and previous events.
	a2, e := New(root, "", "")
	if e != nil {
		t.Fatalf("ledger could not reload: %v", e)
	}
	if len(a2.events) != len(a.events) {
		t.Fatal("ledger event count changed")
	}
	ledger := filepath.Join(root, "data", "ledger.jsonl")
	data, e := os.ReadFile(ledger)
	if e != nil {
		t.Fatal(e)
	}
		// Change one recorded field while preserving JSON structure.
	data = bytes.Replace(data, []byte(`"path":"notes/demo.txt"`), []byte(`"path":"notes/other.txt"`), 1)
	if e := os.WriteFile(ledger, data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(root, "", ""); e == nil {
		t.Fatal("ledger tampering not detected")
	}
}
func TestSafety(t *testing.T) {
	for _, q := range []Request{{Tool: "shell", Path: "rm -rf /"}, {Tool: "write", Path: "../../etc/passwd"}, {Tool: "write", Path: "/tmp/x"}, {Tool: "write", Path: "safe/x", Content: strings.Repeat("z", 131073)}} {
		if d := decision(q); d == "ALLOW" {
			t.Fatalf("unsafe request allowed %+v", q)
		}
	}
	for _, q := range []Request{{Tool: "write", Path: "a/b", Content: "hi"}, {Tool: "read", Path: "a/b"}, {Tool: "list", Path: ""}} {
		if d := decision(q); d != "ALLOW" {
			t.Fatalf("safe request denied: %s", d)
		}
	}
	a, e := New(t.TempDir(), "https://example.com/v1/chat/completions", "")
	if e == nil || a != nil {
		t.Fatal("remote model URL was accepted")
	}
}
func TestInferenceRoute(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "wrong route", 404)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"local response"}}]}`)
	}))
	defer mock.Close()
	a, e := New(t.TempDir(), mock.URL+"/v1/chat/completions", "local-model")
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	status, b := request(t, srv.Client(), "POST", srv.URL+"/api/infer", a.TokenHex(), map[string]string{"prompt": "request"})
	expect(t, status, 200, b)
	if payload(t, b)["response"] != "local response" {
		t.Fatal("wrong model response")
	}
	status, b = request(t, srv.Client(), "GET", srv.URL+"/api/events", a.TokenHex(), nil)
	expect(t, status, 200, b)
	if !bytes.Contains(b, []byte(`"kind":"inference"`)) {
		t.Fatal("inference evidence not recorded")
	}
}
func TestCrossSite(t *testing.T) {
	a, e := New(t.TempDir(), "", "")
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	req, e := http.NewRequest("POST", srv.URL+"/api/execute", strings.NewReader(`{"tool":"list","path":""}`))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Authorization", "Bearer "+a.TokenHex())
	resp, e := srv.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("cross-origin request status: %d", resp.StatusCode)
	}
}

func copyAssets(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"index.html", "live.js", "live.css"} {
		b, e := os.ReadFile(filepath.Join("..", "..", name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(root, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
