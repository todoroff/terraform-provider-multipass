package multipasscli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

// Reuse the test executable as a portable CLI subprocess. No real VMs are touched.
func TestMain(m *testing.M) {
	if os.Getenv("MULTIPASS_TEST_ECHO_ENV") == "1" {
		echoCommandEnvironment()
		os.Exit(0)
	}
	if logPath := os.Getenv("MULTIPASS_TEST_COMMAND_LOG"); logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(2)
		}
		err = json.NewEncoder(f).Encode(os.Args[1:])
		f.Close()
		if err != nil {
			os.Exit(2)
		}
		var responses map[string]struct {
			Stdout   string
			Stderr   string
			ExitCode int
		}
		if raw := os.Getenv("MULTIPASS_TEST_RESPONSES"); raw != "" {
			if json.Unmarshal([]byte(raw), &responses) != nil {
				os.Exit(2)
			}
		}
		response := responses[strings.Join(os.Args[1:], " ")]
		fmt.Fprint(os.Stdout, response.Stdout)
		fmt.Fprint(os.Stderr, response.Stderr)
		if response.ExitCode != 0 {
			os.Exit(response.ExitCode)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReadOnlyMountRejectedBeforeCLI(t *testing.T) {
	for _, operation := range []string{"launch", "mount"} {
		t.Run(operation, func(t *testing.T) {
			c, logPath := recordingClient(t)
			mount := models.Mount{HostPath: "/host", InstancePath: "/workspace", ReadOnly: true}
			var err error
			if operation == "launch" {
				err = c.LaunchInstance(context.Background(), models.LaunchOptions{Name: "vm", Mounts: []models.Mount{mount}})
			} else {
				err = c.Mount(context.Background(), "vm", mount)
			}
			if err == nil {
				t.Fatal("read-only mount must be rejected")
			}
			if _, err := os.Stat(logPath); !os.IsNotExist(err) {
				t.Fatal("CLI was invoked for unsupported mount")
			}
		})
	}
}

func recordingClient(t *testing.T) (*client, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "commands.jsonl")
	t.Setenv("MULTIPASS_TEST_COMMAND_LOG", logPath)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return &client{binaryPath: binary, timeout: 5 * time.Second}, logPath
}

func TestDeleteInstanceScopesPurge(t *testing.T) {
	for _, purge := range []bool{false, true} {
		t.Run(map[bool]string{false: "soft", true: "permanent"}[purge], func(t *testing.T) {
			c, logPath := recordingClient(t)
			if err := c.DeleteInstance(context.Background(), "managed-vm", purge); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			decoder := json.NewDecoder(f)
			var got []string
			if err := decoder.Decode(&got); err != nil {
				t.Fatal(err)
			}
			want := []string{"delete", "managed-vm"}
			if purge {
				want = []string{"delete", "--purge", "managed-vm"}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("command = %v, want %v", got, want)
			}
			if decoder.More() {
				t.Fatal("unexpected additional command; global purge must never be used")
			}
		})
	}
}
