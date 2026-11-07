// Package harness runs the local, authenticated TitanU Harness control plane.
// Its tools are bounded workspace file operations; it never invokes a shell.
package harness

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxBody = 256 << 10

type Event struct {
	ID         string `json:"id"`
	Time       string `json:"time"`
	Kind       string `json:"kind"`
	Tool       string `json:"tool,omitempty"`
	Path       string `json:"path,omitempty"`
	Allowed    bool   `json:"allowed"`
	Decision   string `json:"decision"`
	Before     string `json:"before,omitempty"`
	After      string `json:"after,omitempty"`
	OutputHash string `json:"output_hash,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Chosen     string `json:"chosen,omitempty"`
	Rejected   string `json:"rejected,omitempty"`
	Prev       string `json:"prev"`
	Hash       string `json:"hash"`
	MAC        string `json:"mac"`
}

type Request struct {
	Tool    string `json:"tool"`
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
}

type App struct {
	Root     string
	dir      string
	key      []byte
	token    string
	mu       sync.Mutex
	events   []Event
	prev     string
	sessions map[string]time.Time
	modelURL string
	modelID  string
	client   *http.Client
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func sum(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }
func secureFile(path string, n int) ([]byte, error) {
	if b, e := os.ReadFile(path); e == nil {
		if len(b) != n {
			return nil, fmt.Errorf("invalid secret length: %s", path)
		}
		return b, nil
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return nil, e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return nil, e
	}
	return b, f.Close()
}
func New(root, modelURL, modelID string) (*App, error) {
	abs, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	for _, dir := range []string{filepath.Join(abs, "data"), filepath.Join(abs, "data", "workspace")} {
		if e := os.MkdirAll(dir, 0700); e != nil {
			return nil, e
		}
	}
	data := filepath.Join(abs, "data")
	k, e := secureFile(filepath.Join(data, "ledger.key"), 32)
	if e != nil {
		return nil, e
	}
	t, e := secureFile(filepath.Join(data, "control.token"), 32)
	if e != nil {
		return nil, e
	}
	if modelID == "" {
		modelID = "local"
	}
	a := &App{Root: abs, dir: data, key: k, token: hex.EncodeToString(t), sessions: make(map[string]time.Time), modelID: modelID, client: &http.Client{Timeout: 15 * time.Second}}
	if modelURL != "" {
		u, e := url.Parse(modelURL)
		if e != nil {
			return nil, e
		}
		host := u.Hostname()
		if u.Scheme != "http" || !isLoopback(host) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1/chat/completions" {
			return nil, errors.New("model URL must be a local http://127.0.0.1:port/v1/chat/completions or localhost equivalent")
		}
		a.modelURL = u.String()
	}
	if e := a.loadLedger(); e != nil {
		return nil, e
	}
	return a, nil
}
func isLoopback(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
func (a *App) TokenPath() string       { return filepath.Join(a.dir, "control.token") }
func (a *App) TokenHex() string        { return a.token }
func (a *App) ledgerPath() string      { return filepath.Join(a.dir, "ledger.jsonl") }
func (a *App) payload(ev Event) []byte { ev.Hash = ""; ev.MAC = ""; b, _ := json.Marshal(ev); return b }
func (a *App) seal(ev *Event) {
	ev.Hash = sum(a.payload(*ev))
	h := hmac.New(sha256.New, a.key)
	h.Write([]byte(ev.Hash))
	ev.MAC = hex.EncodeToString(h.Sum(nil))
}
func (a *App) verifyAll(events []Event) error {
	p := ""
	for i, ev := range events {
		if ev.Prev != p {
			return fmt.Errorf("ledger line %d: previous hash mismatch", i+1)
		}
		copyEv := ev
		a.seal(&copyEv)
		if subtle.ConstantTimeCompare([]byte(copyEv.Hash), []byte(ev.Hash)) != 1 || subtle.ConstantTimeCompare([]byte(copyEv.MAC), []byte(ev.MAC)) != 1 {
			return fmt.Errorf("ledger line %d: MAC/hash verification failed", i+1)
		}
		p = ev.Hash
	}
	return nil
}
func (a *App) loadLedger() error {
	b, e := os.ReadFile(a.ledgerPath())
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		return errors.New("ledger has incomplete final record")
	}
	for i, line := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if e := json.Unmarshal(line, &ev); e != nil {
			return fmt.Errorf("ledger line %d: %w", i+1, e)
		}
		a.events = append(a.events, ev)
	}
	if e := a.verifyAll(a.events); e != nil {
		return e
	}
	if len(a.events) > 0 {
		a.prev = a.events[len(a.events)-1].Hash
	}
	return nil
}
func (a *App) checkLedger() error { // Caller holds a.mu. Always compare with persisted bytes, not cached evidence.
	b, e := os.ReadFile(a.ledgerPath())
	if os.IsNotExist(e) {
		if len(a.events) == 0 {
			return nil
		}
		return errors.New("LEDGER_MISSING")
	}
	if e != nil {
		return e
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		return errors.New("LEDGER_INCOMPLETE")
	}
	var onDisk []Event
	for _, line := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			return errors.New("LEDGER_BLANK_LINE")
		}
		var ev Event
		if e := json.Unmarshal(line, &ev); e != nil {
			return e
		}
		onDisk = append(onDisk, ev)
	}
	if e := a.verifyAll(onDisk); e != nil {
		return e
	}
	if len(onDisk) != len(a.events) {
		return errors.New("LEDGER_LENGTH_CHANGED")
	}
	if len(onDisk) > 0 && onDisk[len(onDisk)-1].Hash != a.prev {
		return errors.New("LEDGER_HEAD_CHANGED")
	}
	return nil
}

func (a *App) record(ev Event) (Event, error) { // Caller holds a.mu.
	if e := a.checkLedger(); e != nil {
		return Event{}, fmt.Errorf("LEDGER_INTEGRITY_FAILED: %w", e)
	}
	id, e := randomHex(12)
	if e != nil {
		return Event{}, e
	}
	ev.ID = id
	ev.Time = time.Now().UTC().Format(time.RFC3339Nano)
	ev.Prev = a.prev
	a.seal(&ev)
	line, e := json.Marshal(ev)
	if e != nil {
		return Event{}, e
	}
	f, e := os.OpenFile(a.ledgerPath(), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if e != nil {
		return Event{}, e
	}
	if _, e = f.Write(append(line, '\n')); e != nil {
		f.Close()
		return Event{}, e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return Event{}, e
	}
	if e = f.Close(); e != nil {
		return Event{}, e
	}
	a.events = append(a.events, ev)
	a.prev = ev.Hash
	return ev, nil
}
func pathParts(path string, allowRoot bool) ([]string, error) {
	if allowRoot && (path == "" || path == ".") {
		return nil, nil
	}
	if len(path) == 0 || len(path) > 200 || strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.ContainsRune(path, 0) {
		return nil, errors.New("INVALID_PATH")
	}
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || len(p) > 100 {
			return nil, errors.New("INVALID_PATH")
		}
	}
	return parts, nil
}
func authority(q Request) string {
	if q.Tool != "read" && q.Tool != "hash" && q.Tool != "list" && q.Tool != "write" {
		return "DANGEROUS_COMMAND"
	}
	if _, e := pathParts(q.Path, q.Tool == "list"); e != nil {
		return "INVALID_PATH"
	}
	switch q.Tool {
	case "read", "hash", "list":
		if q.Content != "" {
			return "UNEXPECTED_CONTENT"
		}
		return "ALLOW"
	case "write":
		if len(q.Content) > 128<<10 {
			return "CONTENT_TOO_LARGE"
		}
		return "ALLOW"
	default:
		return "DANGEROUS_COMMAND"
	}
}
func mirror(q Request) string { // Independent policy implementation; disagreement fails closed.
	allowed := q.Tool == "read" || q.Tool == "hash" || q.Tool == "list" || q.Tool == "write"
	if !allowed {
		return "DANGEROUS_COMMAND"
	}
	if q.Path == "" && q.Tool != "list" {
		return "INVALID_PATH"
	}
	if _, e := pathParts(q.Path, q.Tool == "list"); e != nil {
		return "INVALID_PATH"
	}
	if q.Tool != "write" && len(q.Content) != 0 {
		return "UNEXPECTED_CONTENT"
	}
	if q.Tool == "write" && len(q.Content) > 131072 {
		return "CONTENT_TOO_LARGE"
	}
	return "ALLOW"
}
func decision(q Request) string {
	a, b := authority(q), mirror(q)
	if a != b {
		return "POLICY_DIVERGENCE"
	}
	return a
}
func (a *App) workRoot() string { return filepath.Join(a.dir, "workspace") }
func traverse(root string, parts []string, makeDirs bool) (int, error) {
	fd, e := syscall.Open(root, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return -1, e
	}
	for _, p := range parts {
		if makeDirs {
			if e = syscall.Mkdirat(fd, p, 0700); e != nil && e != syscall.EEXIST {
				syscall.Close(fd)
				return -1, e
			}
		}
		next, e := syscall.Openat(fd, p, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}
func safeRead(root, path string) ([]byte, error) {
	parts, e := pathParts(path, false)
	if e != nil {
		return nil, e
	}
	fd, e := traverse(root, parts[:len(parts)-1], false)
	if e != nil {
		return nil, e
	}
	defer syscall.Close(fd)
	fileFD, e := syscall.Openat(fd, parts[len(parts)-1], syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fileFD), "safe-file")
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("NOT_REGULAR_FILE")
	}
	if st.Size() > 128<<10 {
		return nil, errors.New("FILE_TOO_LARGE")
	}
	return io.ReadAll(io.LimitReader(f, (128<<10)+1))
}
func safeList(root, path string) ([]string, error) {
	parts, e := pathParts(path, true)
	if e != nil {
		return nil, e
	}
	fd, e := traverse(root, parts, false)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "directory")
	defer f.Close()
	names, e := f.Readdirnames(101)
	if e != nil {
		return nil, e
	}
	if len(names) > 100 {
		return nil, errors.New("TOO_MANY_FILES")
	}
	return names, nil
}
func safeWrite(root, path, content string) error {
	parts, e := pathParts(path, false)
	if e != nil {
		return e
	}
	fd, e := traverse(root, parts[:len(parts)-1], true)
	if e != nil {
		return e
	}
	defer syscall.Close(fd)
	tmp, e := randomHex(12)
	if e != nil {
		return e
	}
	tmp = ".tmp-" + tmp
	fileFD, e := syscall.Openat(fd, tmp, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fileFD), "new-file")
	_, e = io.WriteString(f, content)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		syscall.Unlinkat(fd, tmp)
		return e
	}
	// Rename replaces the directory entry, never follows a symlink target.
	e = syscall.Renameat(fd, tmp, fd, parts[len(parts)-1])
	if e != nil {
		syscall.Unlinkat(fd, tmp)
		return e
	}
	return syscall.Fsync(fd)
}
func (a *App) execute(q Request) (map[string]any, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if e := a.checkLedger(); e != nil {
		return map[string]any{"error": "LEDGER_INTEGRITY_FAILED", "detail": e.Error()}, 409
	}
	dec := decision(q)
	ev := Event{Kind: "execution", Tool: q.Tool, Path: q.Path, Allowed: dec == "ALLOW", Decision: dec}
	if dec != "ALLOW" {
		stored, e := a.record(ev)
		if e != nil {
			return map[string]any{"error": e.Error()}, 500
		}
		return map[string]any{"error": dec, "id": stored.ID}, 403
	}
	var out any
	var e error
	var before, after string
	if q.Tool == "write" {
		if b, err := safeRead(a.workRoot(), q.Path); err == nil {
			before = sum(b)
		}
		e = safeWrite(a.workRoot(), q.Path, q.Content)
		if e == nil {
			after = sum([]byte(q.Content))
			out = map[string]any{"sha256": after, "bytes": len(q.Content)}
		}
	} else if q.Tool == "read" || q.Tool == "hash" {
		var b []byte
		b, e = safeRead(a.workRoot(), q.Path)
		if e == nil {
			after = sum(b)
			if q.Tool == "read" {
				out = map[string]any{"content": string(b), "sha256": after}
			} else {
				out = map[string]any{"sha256": after, "bytes": len(b)}
			}
		}
	} else if q.Tool == "list" {
		out, e = safeList(a.workRoot(), q.Path)
	}
	if e != nil {
		ev.Allowed = false
		ev.Decision = "TOOL_ERROR"
		ev.OutputHash = sum([]byte(e.Error()))
	} else {
		ev.After = after
		if out != nil {
			b, _ := json.Marshal(out)
			ev.OutputHash = sum(b)
		}
	}
	ev.Before = before
	stored, le := a.record(ev)
	if le != nil {
		return map[string]any{"error": "LEDGER_WRITE_FAILED", "detail": le.Error()}, 500
	}
	if e != nil {
		return map[string]any{"error": "TOOL_ERROR", "detail": e.Error(), "id": stored.ID}, 422
	}
	return map[string]any{"id": stored.ID, "decision": "ALLOW", "result": out, "sha256": after}, 200
}
func write(w http.ResponseWriter, status int, val any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(val)
}
func decode(r *http.Request, out any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return errors.New("application/json required")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
func (a *App) hasSession(r *http.Request) bool {
	v := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if v != "" && subtle.ConstantTimeCompare([]byte(v), []byte(a.token)) == 1 {
		return true
	}
	c, e := r.Cookie("titanu_session")
	if e != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[c.Value]
	return ok && time.Now().Before(exp)
}
func csrf(r *http.Request) bool {
	if v := r.Header.Get("Sec-Fetch-Site"); v == "cross-site" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, e := url.Parse(o)
		if e != nil || u.Scheme != "http" || u.Host != r.Host {
			return false
		}
	} else if r.Header.Get("Authorization") == "" {
		return false
	}
	return true
}
func (a *App) Serve(addr string) error {
	s := &http.Server{Addr: addr, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	return s.ListenAndServe()
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	assets := map[string]string{"/": "index.html", "/live.js": "live.js", "/live.css": "live.css"}
	for path, name := range assets {
		path, name := path, name
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				http.NotFound(w, r)
				return
			}
			b, e := os.ReadFile(filepath.Join(a.Root, name))
			if e != nil {
				http.Error(w, "asset missing", 500)
				return
			}
			typ := "text/html; charset=utf-8"
			if strings.HasSuffix(name, ".js") {
				typ = "application/javascript; charset=utf-8"
			}
			if strings.HasSuffix(name, ".css") {
				typ = "text/css; charset=utf-8"
			}
			w.Header().Set("Content-Type", typ)
			w.Header().Set("Cache-Control", "no-store")
			w.Write(b)
		})
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"ok": true, "service": "titanu-harness", "mode": "local"})
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Token string `json:"token"`
		}
		if e := decode(r, &q); e != nil {
			write(w, 400, map[string]any{"error": e.Error()})
			return
		}
		if subtle.ConstantTimeCompare([]byte(q.Token), []byte(a.token)) != 1 {
			write(w, 401, map[string]any{"error": "UNAUTHORIZED"})
			return
		}
		s, e := randomHex(32)
		if e != nil {
			write(w, 500, map[string]any{"error": e.Error()})
			return
		}
		a.mu.Lock()
		a.sessions[s] = time.Now().Add(12 * time.Hour)
		a.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "titanu_session", Value: s, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
		write(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, e := r.Cookie("titanu_session"); e == nil {
			a.mu.Lock()
			delete(a.sessions, c.Value)
			a.mu.Unlock()
		}
		http.SetCookie(w, &http.Cookie{Name: "titanu_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		write(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		allowed, denied := 0, 0
		for _, ev := range a.events {
			if ev.Kind == "execution" {
				if ev.Allowed {
					allowed++
				} else {
					denied++
				}
			}
		}
		write(w, 200, map[string]any{"total": len(a.events), "allowed": allowed, "denied": denied, "head": a.prev, "model_configured": a.modelURL != "", "policy_authority": "go", "proof": "HMAC-SHA256"})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		n := len(a.events)
		start := n - 100
		if start < 0 {
			start = 0
		}
		items := append([]Event{}, a.events[start:]...)
		write(w, 200, map[string]any{"events": items})
	})
	mux.HandleFunc("GET /api/verify", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		e := a.checkLedger()
		if e != nil {
			write(w, 409, map[string]any{"verified": false, "error": e.Error()})
			return
		}
		write(w, 200, map[string]any{"verified": true, "events": len(a.events), "head": a.prev})
	})
	mux.HandleFunc("POST /api/evaluate", func(w http.ResponseWriter, r *http.Request) {
		var q Request
		if e := decode(r, &q); e != nil {
			write(w, 400, map[string]any{"error": e.Error()})
			return
		}
		d := decision(q)
		write(w, 200, map[string]any{"allowed": d == "ALLOW", "decision": d})
	})
	mux.HandleFunc("POST /api/execute", func(w http.ResponseWriter, r *http.Request) {
		var q Request
		if e := decode(r, &q); e != nil {
			write(w, 400, map[string]any{"error": e.Error()})
			return
		}
		v, status := a.execute(q)
		write(w, status, v)
	})
	mux.HandleFunc("GET /api/replay/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, ev := range a.events {
			if ev.ID != id {
				continue
			}
			now := ""
			if ev.Path != "" && (ev.Tool == "read" || ev.Tool == "hash" || ev.Tool == "write") {
				if b, e := safeRead(a.workRoot(), ev.Path); e == nil {
					now = sum(b)
				}
			}
			write(w, 200, map[string]any{"event": ev, "current_sha256": now, "matches_recorded_after": now != "" && now == ev.After, "note": "read-only evidence replay; no commands rerun"})
			return
		}
		write(w, 404, map[string]any{"error": "EVENT_NOT_FOUND"})
	})
	mux.HandleFunc("POST /api/feedback", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Prompt   string `json:"prompt"`
			Chosen   string `json:"chosen"`
			Rejected string `json:"rejected"`
		}
		if e := decode(r, &q); e != nil {
			write(w, 400, map[string]any{"error": e.Error()})
			return
		}
		if q.Prompt == "" || q.Chosen == "" || q.Rejected == "" || len(q.Prompt) > 2048 || len(q.Chosen) > 2048 || len(q.Rejected) > 2048 || q.Chosen == q.Rejected {
			write(w, 400, map[string]any{"error": "INVALID_FEEDBACK"})
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		ev, e := a.record(Event{Kind: "preference", Allowed: true, Decision: "RECORDED", Prompt: q.Prompt, Chosen: q.Chosen, Rejected: q.Rejected})
		if e != nil {
			write(w, 500, map[string]any{"error": e.Error()})
			return
		}
		write(w, 201, map[string]any{"id": ev.ID})
	})
	mux.HandleFunc("GET /api/export/{kind}", func(w http.ResponseWriter, r *http.Request) {
		kind := r.PathValue("kind")
		if kind != "sft" && kind != "preferences" {
			write(w, 404, map[string]any{"error": "UNKNOWN_EXPORT"})
			return
		}
		a.mu.Lock()
		events := append([]Event{}, a.events...)
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.jsonl\"", kind))
		w.Header().Set("Cache-Control", "no-store")
		enc := json.NewEncoder(w)
		for _, ev := range events {
			if kind == "sft" && ev.Kind == "execution" {
				_ = enc.Encode(map[string]any{"instruction": map[string]any{"tool": ev.Tool, "path": ev.Path}, "response": map[string]any{"allowed": ev.Allowed, "decision": ev.Decision, "after_sha256": ev.After}, "source_event": ev.ID})
			}
			if kind == "preferences" && ev.Kind == "preference" {
				_ = enc.Encode(map[string]any{"prompt": ev.Prompt, "chosen": ev.Chosen, "rejected": ev.Rejected, "source_event": ev.ID})
			}
		}
	})
	mux.HandleFunc("POST /api/infer", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Prompt string `json:"prompt"`
		}
		if e := decode(r, &q); e != nil {
			write(w, 400, map[string]any{"error": e.Error()})
			return
		}
		if strings.TrimSpace(q.Prompt) == "" || len(q.Prompt) > 4096 {
			write(w, 400, map[string]any{"error": "INVALID_PROMPT"})
			return
		}
		if a.modelURL == "" {
			write(w, 503, map[string]any{"error": "MODEL_NOT_CONFIGURED"})
			return
		}
		payload, _ := json.Marshal(map[string]any{"model": a.modelID, "messages": []map[string]string{{"role": "user", "content": q.Prompt}}, "max_tokens": 256, "stream": false})
		req, e := http.NewRequestWithContext(r.Context(), "POST", a.modelURL, bytes.NewReader(payload))
		if e != nil {
			write(w, 500, map[string]any{"error": e.Error()})
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, e := a.client.Do(req)
		if e != nil {
			write(w, 502, map[string]any{"error": "MODEL_UNAVAILABLE", "detail": e.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			write(w, 502, map[string]any{"error": "MODEL_BAD_STATUS", "upstream_status": resp.StatusCode})
			return
		}
		var o struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if e := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&o); e != nil || len(o.Choices) == 0 {
			write(w, 502, map[string]any{"error": "MODEL_INVALID_RESPONSE"})
			return
		}
		reply := o.Choices[0].Message.Content
		a.mu.Lock()
		ev, le := a.record(Event{Kind: "inference", Allowed: true, Decision: "MODEL_RESPONSE", OutputHash: sum([]byte(reply))})
		a.mu.Unlock()
		if le != nil {
			write(w, 500, map[string]any{"error": le.Error()})
			return
		}
		write(w, 200, map[string]any{"id": ev.ID, "response": reply})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if !isLoopback(host) {
			http.Error(w, "LOOPBACK_ONLY", 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !csrf(r) {
			write(w, 403, map[string]any{"error": "CROSS_ORIGIN_BLOCKED"})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/login" && r.URL.Path != "/api/logout" && !a.hasSession(r) {
			write(w, 401, map[string]any{"error": "UNAUTHORIZED"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
