package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// testBinaryPath holds the path of the annotate binary built once in TestMain.
var testBinaryPath string

// TestMain builds the annotate binary once and runs all tests against it.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "annotate-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}
	testBinaryPath = filepath.Join(tmp, "annotate")

	build := exec.Command("go", "build", "-o", testBinaryPath, "github.com/briansurratt/annotate/cmd/annotate")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.RemoveAll(tmp)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// runAnnotate runs the annotate binary with the given args.
// It returns stdout, stderr, and the exit code.
func runAnnotate(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(testBinaryPath, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return
}

// writeTestConfig writes a minimal valid config YAML and returns its path and the socket path.
// The socket path uses os.MkdirTemp to stay under macOS's 104-byte sun_path limit.
func writeTestConfig(t *testing.T) (cfgPath, socketPath string) {
	t.Helper()
	workspace := t.TempDir()

	sockDir, err := os.MkdirTemp("", "ann-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	socketPath = filepath.Join(sockDir, "a.sock")

	cfgContent := fmt.Sprintf("workspace_path: %s\nsocket_path: %s\n", workspace, socketPath)
	cfgDir := t.TempDir()
	cfgPath = filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath, socketPath
}

// waitForSocket polls until the socket file appears or the deadline passes.
func waitForSocket(t *testing.T, socketPath string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ─── Section 2: Root command ─────────────────────────────────────────────────

// TestHelp_ListsAllSubcommands verifies that --help output includes all six subcommand names.
func TestHelp_ListsAllSubcommands(t *testing.T) {
	stdout, _, _ := runAnnotate(t, "--help")
	for _, name := range []string{"daemon", "enqueue", "apply", "report", "status", "index"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("--help output missing subcommand %q\nfull output:\n%s", name, stdout)
		}
	}
}

// TestUnknownSubcommand_ExitsNonZero verifies unknown subcommands exit non-zero.
func TestUnknownSubcommand_ExitsNonZero(t *testing.T) {
	_, stderr, code := runAnnotate(t, "unknown-cmd")
	if code == 0 {
		t.Error("unknown subcommand: want non-zero exit code, got 0")
	}
	if stderr == "" {
		t.Error("unknown subcommand: want error message on stderr, got empty")
	}
}

// ─── Section 3: daemon subcommand ────────────────────────────────────────────

// TestDaemon_StartsAndExitsZeroOnSIGTERM verifies the daemon starts with a valid
// config and exits 0 after receiving SIGTERM.
func TestDaemon_StartsAndExitsZeroOnSIGTERM(t *testing.T) {
	cfgPath, socketPath := writeTestConfig(t)

	cmd := exec.Command(testBinaryPath, "--config", cfgPath, "daemon")
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	if !waitForSocket(t, socketPath, 5*time.Second) {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("daemon socket not created within 5s at %s", socketPath)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() != 0 {
					t.Errorf("daemon exit code = %d, want 0", exitErr.ExitCode())
				}
			} else {
				t.Errorf("wait: %v", err)
			}
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("daemon did not exit within 10s after SIGTERM")
	}
}

// TestDaemon_MissingConfigExitsNonZero verifies that a missing config file causes
// a non-zero exit with an error message on stderr.
func TestDaemon_MissingConfigExitsNonZero(t *testing.T) {
	_, stderr, code := runAnnotate(t, "--config", "/nonexistent/path/config.yaml", "daemon")
	if code == 0 {
		t.Error("daemon with missing config: want non-zero exit code, got 0")
	}
	if stderr == "" {
		t.Error("daemon with missing config: want error message on stderr, got empty")
	}
}

// ─── Section 4: enqueue subcommand ───────────────────────────────────────────

// TestEnqueue_WithDaemonRunning_ExitsZero verifies enqueue exits 0 when the daemon is running.
func TestEnqueue_WithDaemonRunning_ExitsZero(t *testing.T) {
	cfgPath, socketPath := writeTestConfig(t)

	daemonProc := exec.Command(testBinaryPath, "--config", cfgPath, "daemon")
	daemonProc.Stderr = os.Stderr
	if err := daemonProc.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		daemonProc.Process.Signal(syscall.SIGTERM)
		daemonProc.Wait()
	})

	if !waitForSocket(t, socketPath, 5*time.Second) {
		t.Fatalf("daemon socket not created within 5s at %s", socketPath)
	}

	f, err := os.CreateTemp(t.TempDir(), "note-*.md")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	_, stderr, code := runAnnotate(t, "--config", cfgPath, "enqueue", f.Name())
	if code != 0 {
		t.Errorf("enqueue with daemon running: exit code = %d, want 0\nstderr: %s", code, stderr)
	}
}

// TestEnqueue_WithDaemonNotRunning_ExitsNonZero verifies enqueue exits non-zero
// and prints "daemon is not running" to stderr when no daemon is reachable.
func TestEnqueue_WithDaemonNotRunning_ExitsNonZero(t *testing.T) {
	cfgPath, _ := writeTestConfig(t)

	_, stderr, code := runAnnotate(t, "--config", cfgPath, "enqueue", "/some/file.md")
	if code == 0 {
		t.Error("enqueue with no daemon: want non-zero exit code, got 0")
	}
	if !strings.Contains(stderr, "daemon is not running") {
		t.Errorf("stderr %q does not contain 'daemon is not running'", stderr)
	}
}

// ─── Section 5: stub subcommands ─────────────────────────────────────────────

func TestApply_PrintsNotYetImplemented(t *testing.T) {
	stdout, _, code := runAnnotate(t, "apply", "somefile.md")
	if code != 0 {
		t.Errorf("apply: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "not yet implemented") {
		t.Errorf("apply stdout %q does not contain 'not yet implemented'", stdout)
	}
}

func TestReport_PrintsNotYetImplemented(t *testing.T) {
	stdout, _, code := runAnnotate(t, "report")
	if code != 0 {
		t.Errorf("report: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "not yet implemented") {
		t.Errorf("report stdout %q does not contain 'not yet implemented'", stdout)
	}
}

func TestStatus_PrintsNotYetImplemented(t *testing.T) {
	stdout, _, code := runAnnotate(t, "status")
	if code != 0 {
		t.Errorf("status: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "not yet implemented") {
		t.Errorf("status stdout %q does not contain 'not yet implemented'", stdout)
	}
}

func TestIndexRebuild_PrintsNotYetImplemented(t *testing.T) {
	stdout, _, code := runAnnotate(t, "index", "rebuild")
	if code != 0 {
		t.Errorf("index rebuild: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "not yet implemented") {
		t.Errorf("index rebuild stdout %q does not contain 'not yet implemented'", stdout)
	}
}
