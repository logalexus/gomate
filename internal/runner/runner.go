// Package runner executes user-authored scripts in disposable, fixed-policy Docker containers.
package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MaxCodeBytes      = 256 << 10
	MaxOutputBytes    = 512 << 10
	MaxTimeoutSeconds = 120
	MaxConcurrent     = 2
	maxJobs           = 32
	resultTTL         = 15 * time.Minute
	owner             = "gomate-script-runner-v1"
)

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var containerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Request struct {
	Language       string `json:"language"`
	Code           string `json:"code"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}
type Result struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  *int   `json:"exitCode"`
	ElapsedMS int64  `json:"elapsedMs"`
	Truncated bool   `json:"truncated"`
	Error     string `json:"error,omitempty"`
}
type job struct {
	result            Result
	stdout, stderr    []byte
	started, finished time.Time
	cancel            context.CancelFunc
	canceled          bool
}
type Manager struct {
	client *http.Client
	image  string
	mu     sync.Mutex
	jobs   map[string]*job
	active int
	ready  bool
}
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

func New(socket, image string) *Manager {
	if image == "" {
		image = "gomate-runner:latest"
	}
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
	}, MaxIdleConns: 8, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second}
	return &Manager{client: &http.Client{Transport: tr}, image: image, jobs: make(map[string]*job)}
}
func (m *Manager) call(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(r)
	if err != nil {
		return nil, &Error{503, "Docker-шлюз исполнения недоступен"}
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, &Error{resp.StatusCode, fmt.Sprintf("Docker вернул HTTP %d", resp.StatusCode)}
	}
	return resp, nil
}
func readJSON(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if e != nil {
		return e
	}
	if len(b) > 2<<20 {
		return errors.New("oversized Docker response")
	}
	return json.Unmarshal(b, dst)
}
func (m *Manager) discard(ctx context.Context, method, path string) error {
	resp, e := m.call(ctx, method, path, nil)
	if resp != nil {
		resp.Body.Close()
	}
	return e
}
func dockerMissing(err error) bool { var e *Error; return errors.As(err, &e) && e.Code == 404 }

// Initialize removes only this runner's containers left behind by a gateway restart.
// Failure disables new executions instead of potentially exceeding concurrency limits.
func (m *Manager) Initialize(ctx context.Context) error {
	filters, _ := json.Marshal(map[string][]string{"label": {"gomate.runner.owner=" + owner}})
	resp, e := m.call(ctx, "GET", "/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil)
	if e != nil {
		return e
	}
	var cs []struct {
		ID     string            `json:"Id"`
		Labels map[string]string `json:"Labels"`
	}
	if e = readJSON(resp, &cs); e != nil {
		return e
	}
	for _, c := range cs {
		if c.Labels["gomate.runner.owner"] != owner || !containerID.MatchString(c.ID) {
			continue
		}
		if e = m.remove(ctx, c.ID); e != nil {
			return e
		}
	}
	m.mu.Lock()
	m.ready = true
	m.mu.Unlock()
	return nil
}
func (m *Manager) remove(ctx context.Context, id string) error {
	e := m.discard(ctx, "DELETE", "/containers/"+id+"?force=1&v=1")
	if dockerMissing(e) {
		return nil
	}
	return e
}
func (m *Manager) Available(ctx context.Context) bool {
	m.mu.Lock()
	ready := m.ready
	m.pruneLocked()
	m.mu.Unlock()
	if !ready {
		return false
	}
	resp, e := m.call(ctx, "GET", "/images/"+url.PathEscape(m.image)+"/json", nil)
	if resp != nil {
		resp.Body.Close()
	}
	return e == nil
}
func (m *Manager) pruneLocked() {
	for id, j := range m.jobs {
		if !j.finished.IsZero() && time.Since(j.finished) > resultTTL {
			delete(m.jobs, id)
		}
	}
}
func (m *Manager) snapshotLocked(j *job) Result {
	r := j.result
	r.Stdout = strings.ToValidUTF8(string(j.stdout), "�")
	r.Stderr = strings.ToValidUTF8(string(j.stderr), "�")
	end := j.finished
	if end.IsZero() {
		end = time.Now()
	}
	r.ElapsedMS = end.Sub(j.started).Milliseconds()
	return r
}
func (m *Manager) Get(id string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	j := m.jobs[id]
	if j == nil {
		return Result{}, &Error{404, "Запуск не найден или результат устарел"}
	}
	return m.snapshotLocked(j), nil
}
func (m *Manager) Cancel(id string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil {
		return Result{}, &Error{404, "Запуск не найден"}
	}
	if j.finished.IsZero() {
		j.canceled = true
		j.result.Status = "stopping"
		j.cancel()
	}
	return m.snapshotLocked(j), nil
}
func (m *Manager) Start(req Request) (Result, error) {
	if req.Language != "python" && req.Language != "bash" {
		return Result{}, &Error{400, "Выберите Python или Bash"}
	}
	if strings.TrimSpace(req.Code) == "" || len(req.Code) > MaxCodeBytes {
		return Result{}, &Error{400, "Код должен содержать от 1 до 262144 байт"}
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 30
	}
	if req.TimeoutSeconds < 1 || req.TimeoutSeconds > MaxTimeoutSeconds {
		return Result{}, &Error{400, "Время исполнения: 1–120 секунд"}
	}
	idBytes := make([]byte, 16)
	if _, e := rand.Read(idBytes); e != nil {
		return Result{}, e
	}
	id := hex.EncodeToString(idBytes)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	if !m.ready {
		cancel()
		return Result{}, &Error{503, "Runner не готов: проверьте Docker и перезапустите шлюз"}
	}
	if m.active >= MaxConcurrent {
		cancel()
		return Result{}, &Error{429, "Уже выполняются два скрипта. Дождитесь завершения или остановите один"}
	}
	if len(m.jobs) >= maxJobs {
		var oldest string
		var stamp time.Time
		for k, j := range m.jobs {
			if !j.finished.IsZero() && (oldest == "" || j.finished.Before(stamp)) {
				oldest, stamp = k, j.finished
			}
		}
		if oldest != "" {
			delete(m.jobs, oldest)
		}
	}
	j := &job{result: Result{ID: id, Status: "starting"}, started: time.Now(), cancel: cancel}
	m.jobs[id] = j
	m.active++
	go m.run(ctx, j, req)
	return m.snapshotLocked(j), nil
}
func (m *Manager) containerSpec(req Request) map[string]any {
	// Each argument stays below Linux MAX_ARG_STRLEN; code is a file for useful tracebacks.
	script := "/tmp/script.py"
	execute := `[sys.executable,"-u",p]`
	if req.Language == "bash" {
		script = "/tmp/script.sh"
		execute = `["/bin/bash",p]`
	}
	bootstrap := `import base64,os,sys; p=` + strconv.Quote(script) + `; open(p,"wb").write(base64.b64decode("".join(sys.argv[1:]))); os.execv(`
	if req.Language == "bash" {
		bootstrap += `"/bin/bash",` + execute + `)`
	} else {
		bootstrap += `sys.executable,` + execute + `)`
	}
	cmd := []string{"/usr/bin/timeout", "--signal=KILL", strconv.Itoa(req.TimeoutSeconds) + "s", "python3", "-u", "-c", bootstrap}
	code := base64.StdEncoding.EncodeToString([]byte(req.Code))
	for len(code) > 0 {
		n := min(len(code), 48<<10)
		cmd = append(cmd, code[:n])
		code = code[n:]
	}
	return map[string]any{"Image": m.image, "Cmd": cmd, "Entrypoint": []string{}, "User": "65532:65532", "WorkingDir": "/tmp", "Env": []string{"HOME=/tmp", "TMPDIR=/tmp", "TERM=dumb", "PYTHONDONTWRITEBYTECODE=1", "PYTHONUNBUFFERED=1"}, "OpenStdin": false, "Tty": false, "Labels": map[string]string{"gomate.internal": "true", "gomate.runner": "true", "gomate.runner.owner": owner}, "HostConfig": map[string]any{"NetworkMode": "host", "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "PidsLimit": 64, "Memory": int64(512 << 20), "MemorySwap": int64(512 << 20), "NanoCpus": int64(500000000), "Init": true, "Tmpfs": map[string]string{"/tmp": "rw,nosuid,nodev,size=67108864,mode=1777"}, "LogConfig": map[string]any{"Type": "json-file", "Config": map[string]string{"max-size": "1m", "max-file": "1"}}}}
}
func (m *Manager) run(ctx context.Context, j *job, req Request) {
	defer j.cancel()
	name := "gomate-run-" + j.result.ID
	var runErr error
	var exit *int
	// Resolve cleanup by deterministic name even if create's response was lost.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cleanupErr := m.remove(cleanupCtx, name)
		m.mu.Lock()
		defer m.mu.Unlock()
		j.finished = time.Now()
		j.result.ExitCode = exit
		m.active--
		switch {
		case j.canceled:
			j.result.Status = "canceled"
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			j.result.Status = "timed_out"
		case runErr != nil:
			j.result.Status = "failed"
			j.result.Error = runErr.Error()
		default:
			j.result.Status = "finished"
		}
		if cleanupErr != nil {
			m.ready = false
			j.result.Status = "cleanup_failed"
			j.result.Error = "Не удалось удалить контейнер запуска; проверьте Docker"
		}
	}()
	// Let an in-flight create settle before cleanup, even if the user cancels.
	createCtx, stopCreate := context.WithTimeout(context.Background(), 5*time.Second)
	resp, e := m.call(createCtx, "POST", "/containers/create?name="+name, m.containerSpec(req))
	if e != nil {
		stopCreate()
		runErr = e
		return
	}
	var created struct {
		ID string `json:"Id"`
	}
	if e = readJSON(resp, &created); e != nil {
		stopCreate()
		runErr = e
		return
	}
	stopCreate()
	if !containerID.MatchString(created.ID) {
		runErr = errors.New("Docker returned invalid container ID")
		return
	}
	if ctx.Err() != nil {
		return
	}
	startCtx, stopStart := context.WithTimeout(context.Background(), 5*time.Second)
	e = m.discard(startCtx, "POST", "/containers/"+created.ID+"/start")
	stopStart()
	if e != nil {
		runErr = e
		return
	}
	m.mu.Lock()
	if !j.canceled {
		j.result.Status = "running"
	}
	m.mu.Unlock()
	logCtx, stopLogs := context.WithCancel(context.Background())
	defer stopLogs()
	logDone := make(chan error, 1)
	go func() { logDone <- m.logs(logCtx, created.ID, j) }()
	resp, e = m.call(ctx, "POST", "/containers/"+created.ID+"/wait?condition=not-running", nil)
	if e == nil {
		var state struct {
			StatusCode int                       `json:"StatusCode"`
			Error      *struct{ Message string } `json:"Error"`
		}
		e = readJSON(resp, &state)
		if e == nil {
			exit = &state.StatusCode
			if state.Error != nil && state.Error.Message != "" {
				e = errors.New("Docker did not complete the script")
			}
		}
	}
	runErr = e
	if ctx.Err() != nil {
		killCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = m.discard(killCtx, "POST", "/containers/"+created.ID+"/kill?signal=KILL")
		cancel()
	}
	select {
	case logErr := <-logDone:
		if runErr == nil && logErr != nil {
			runErr = logErr
		}
	case <-time.After(time.Second):
		m.mu.Lock()
		j.result.Truncated = true
		m.mu.Unlock()
		stopLogs()
		<-logDone
	}
}
func (m *Manager) logs(ctx context.Context, id string, j *job) error {
	resp, e := m.call(ctx, "GET", "/containers/"+id+"/logs?stdout=1&stderr=1&follow=1&tail=all", nil)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	header := make([]byte, 8)
	buf := make([]byte, 8192)
	for {
		_, e = io.ReadFull(resp.Body, header)
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		if (header[0] != 1 && header[0] != 2) || header[1] != 0 || header[2] != 0 || header[3] != 0 {
			return errors.New("invalid Docker log frame")
		}
		remaining := int64(binary.BigEndian.Uint32(header[4:]))
		for remaining > 0 {
			n, e := io.ReadFull(resp.Body, buf[:min(int64(len(buf)), remaining)])
			if n > 0 {
				m.mu.Lock()
				room := MaxOutputBytes - len(j.stdout) - len(j.stderr)
				keep := min(room, n)
				if keep < n {
					j.result.Truncated = true
				}
				if header[0] == 1 {
					j.stdout = append(j.stdout, buf[:keep]...)
				} else {
					j.stderr = append(j.stderr, buf[:keep]...)
				}
				m.mu.Unlock()
				if keep < n {
					return nil
				}
				remaining -= int64(n)
			}
			if e != nil {
				return e
			}
		}
	}
}
