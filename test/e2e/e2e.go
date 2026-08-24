// Package e2e holds integration tests for the web server. It is compiled on its
// own (no build tag) so that `go test ./...` sees a valid package and skips the
// tagged tests; set the integration tag to run them: go test -tags=integration ./test/e2e/...
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// repoRoot resolves the repository root from the test/e2e package directory.
func repoRoot() string {
	pwd, err := os.Getwd()
	if err != nil {
		pwd = "."
	}
	return filepath.Dir(filepath.Dir(pwd))
}

// buildBinary compiles the web server into the e2e directory and returns its path.
func buildBinary() (string, error) {
	bin := filepath.Join(repoRoot(), "test", "e2e", "llamacpp-launcher")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/llamacpp-launcher")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build failed: %w\n%s", err, out)
	}
	return bin, nil
}

// serverEnv returns the environment used to launch the server during integration tests.
func serverEnv(modelRoot, configDir string, port int) []string {
	return []string{
		"LLAMA_MODEL_ROOT=" + modelRoot,
		"LLAMA_CONFIG_DIR=" + configDir,
		"LLAMA_SERVICE_NAME=llamacpp-e2e",
		"LLAMA_WEB_HOST=127.0.0.1",
		"LLAMA_WEB_PORT=" + strconv.Itoa(port),
	}
}

// freePort finds a free loopback TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected addr type %T", l.Addr())
	}
	return addr.Port, nil
}

// httpGetJSON performs a GET and decodes the JSON body into dst.
func httpGetJSON(base, path string, dst any) error {
	resp, err := http.Get(base + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// httpPostJSON performs a POST with a JSON body and decodes the response into dst.
func httpPostJSON(base, path string, body any, dst any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, buf)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// waitForHTTP polls /api/version until the server answers or the timeout elapses.
func waitForHTTP(base string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := httpGetJSON(base, "/api/version", new(map[string]any)); err != nil {
			lastErr = err
		} else {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready within %s (last error: %v)", timeout, lastErr)
}
