package multipasscli

import (
	"context"
	"encoding/json"
	"errors"
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
		if dictionary := os.Getenv("MULTIPASS_TEST_ALIAS_DICTIONARY"); dictionary != "" {
			data, err := os.ReadFile(dictionary)
			if err != nil {
				os.Exit(3)
			}
			entries := map[string]bool{}
			if json.Unmarshal(data, &entries) != nil {
				os.Exit(3)
			}
			time.Sleep(100 * time.Millisecond)
			entries[strings.Join(os.Args[1:], " ")] = true
			data, _ = json.Marshal(entries)
			if os.WriteFile(dictionary, data, 0600) != nil {
				os.Exit(3)
			}
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

func TestLaunchNameCollision(t *testing.T) {
	c, _ := recordingClient(t)
	t.Setenv("MULTIPASS_TEST_RESPONSES", `{"launch --name vm --timeout 5":{"Stderr":"launch failed: instance \"vm\" already exists","ExitCode":1}}`)
	if err := c.LaunchInstance(context.Background(), models.LaunchOptions{Name: "vm"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("collision not classified: %v", err)
	}
}

func TestConcurrentAliasWritersAcrossClients(t *testing.T) {
	first, _ := recordingClient(t)
	second := &client{binaryPath: first.binaryPath, timeout: first.timeout, serverAddress: "other-daemon:50051"}
	dictionary := filepath.Join(t.TempDir(), "aliases.json")
	if err := os.WriteFile(dictionary, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTIPASS_TEST_ALIAS_DICTIONARY", dictionary)
	operations := []func() error{
		func() error {
			return first.CreateAlias(context.Background(), models.Alias{Name: "first", Instance: "vm", Command: "bash"})
		},
		func() error {
			return second.CreateAlias(context.Background(), models.Alias{Name: "second", Instance: "vm", Command: "bash"})
		},
		func() error { return first.DeleteAlias(context.Background(), "default.old") },
		func() error { return second.DeleteInstance(context.Background(), "other-vm", true) },
		func() error {
			return first.LaunchInstance(context.Background(), models.LaunchOptions{Name: "blueprint-vm"})
		},
	}
	start := make(chan struct{})
	results := make(chan error, len(operations))
	for _, operation := range operations {
		go func() { <-start; results <- operation() }()
	}
	close(start)
	for range operations {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	data, err := os.ReadFile(dictionary)
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]bool
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(operations) {
		t.Fatalf("concurrent writers lost changes: got %d entries, want %d: %s", len(entries), len(operations), data)
	}
}

func TestAliasWriterWaitRespectsContext(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			c, logPath := recordingClient(t)
			aliasWriters <- struct{}{}
			defer func() { <-aliasWriters }()
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			if err := c.LaunchInstance(ctx, models.LaunchOptions{Name: "vm"}); !errors.Is(err, want) || !errors.Is(err, ErrNotStarted) {
				t.Fatalf("error=%v, want %v", err, want)
			}
			if _, err := os.Stat(logPath); !os.IsNotExist(err) {
				t.Fatal("cancelled writer started the CLI")
			}
		})
	}
}

func TestLaunchNeverStarted(t *testing.T) {
	for _, scenario := range []string{"cancelled", "missing executable", "invalid mounts", "invalid cloud-init"} {
		t.Run(scenario, func(t *testing.T) {
			c, logPath := recordingClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := models.LaunchOptions{Name: "vm"}
			switch scenario {
			case "cancelled":
				cancel()
			case "missing executable":
				c.binaryPath = filepath.Join(t.TempDir(), "missing-multipass")
			case "invalid mounts":
				opts.Mounts = []models.Mount{{ReadOnly: true}}
			case "invalid cloud-init":
				opts.CloudInitInline, opts.CloudInitFile = "inline", "file"
			}
			if err := c.LaunchInstance(ctx, opts); !errors.Is(err, ErrNotStarted) {
				t.Fatalf("pre-start failure not classified: %v", err)
			}
			if _, err := os.Stat(logPath); !os.IsNotExist(err) {
				t.Fatal("CLI started")
			}
		})
	}
}
