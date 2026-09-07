package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The helper is invoked as systemd-run, without contacting a real user manager.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "systemd-run" {
		data, _ := json.Marshal(os.Args[1:])
		if err := os.WriteFile(os.Getenv("DANKSESSION_TEST_ARGS"), data, 0o600); err != nil {
			os.Exit(2)
		}
		if os.Getenv("DANKSESSION_TEST_FAIL") == "1" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLaunchUsesIndependentServiceAndPreservesLiteralArguments(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(dir, "systemd-run")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("DANKSESSION_TEST_ARGS", filepath.Join(dir, "args.json"))
	t.Setenv("DANKSESSION_TEST_SECRET", "private-value")
	t.Setenv("NOTIFY_SOCKET", "/must-not-inherit")
	if err := launch(context.Background(), []string{executable, "$HOME", "one argument"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "args.json"))
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"--user", "--collect", "--service-type=exec", "--expand-environment=no", "--setenv=DANKSESSION_TEST_SECRET"} {
		if !slices.Contains(args, arg) {
			t.Fatalf("missing %s: %v", arg, args)
		}
	}
	if strings.Contains(string(data), "private-value") || slices.Contains(args, "--setenv=NOTIFY_SOCKET") {
		t.Fatal("launch argv leaks environment values or inherits daemon notification socket")
	}
	if !slices.Equal(args[len(args)-4:], []string{"--", executable, "$HOME", "one argument"}) {
		t.Fatalf("arguments changed: %v", args)
	}
	t.Setenv("DANKSESSION_TEST_FAIL", "1")
	if err := launch(context.Background(), []string{executable}); err == nil {
		t.Fatal("failed systemd launch reported success")
	}
}
