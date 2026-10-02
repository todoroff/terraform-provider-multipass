package multipasscli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type commandEnvironment struct {
	Address      string
	AddressCount int
	Sentinel     string
	Stdin        string
	Args         []string
}

// Called by TestMain in a real subprocess, so these tests exercise os/exec's
// environment handling on every supported OS without connecting to a daemon.
func echoCommandEnvironment() {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	result := commandEnvironment{
		Address:  os.Getenv("MULTIPASS_SERVER_ADDRESS"),
		Sentinel: os.Getenv("MULTIPASS_TEST_SENTINEL"),
		Stdin:    string(stdin),
		Args:     os.Args[1:],
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key == "MULTIPASS_SERVER_ADDRESS" || (runtime.GOOS == "windows" && strings.EqualFold(key, "MULTIPASS_SERVER_ADDRESS")) {
			result.AddressCount++
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(2)
	}
}

func TestClientServerAddress(t *testing.T) {
	t.Setenv("MULTIPASS_TEST_ECHO_ENV", "1")
	t.Setenv("MULTIPASS_TEST_SENTINEL", "inherited environment")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, inherited, configured, want string
		unset                             bool
	}{
		{name: "local default", unset: true},
		{name: "environment fallback", inherited: "inherited.example:50051", want: "inherited.example:50051"},
		{name: "explicit override", inherited: "inherited.example:50051", configured: "remote.example:50051", want: "remote.example:50051"},
		{name: "explicit without environment", configured: "remote.example:50051", want: "remote.example:50051", unset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MULTIPASS_SERVER_ADDRESS", tc.inherited)
			if tc.unset {
				if err := os.Unsetenv("MULTIPASS_SERVER_ADDRESS"); err != nil {
					t.Fatal(err)
				}
			}
			cli, err := NewClient(context.Background(), Config{BinaryPath: binary, ServerAddress: tc.configured, Timeout: 10})
			if err != nil {
				t.Fatal(err)
			}
			for _, stdin := range [][]byte{nil, []byte("#cloud-config\npackages: [curl]\n")} {
				args := []string{"launch", "--cloud-init", "-"}
				out, err := cli.(*client).runWithStdin(context.Background(), stdin, args...)
				if err != nil {
					t.Fatal(err)
				}
				var got commandEnvironment
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatal(err)
				}
				wantCount := 1
				if tc.unset && tc.configured == "" {
					wantCount = 0
				}
				if got.Address != tc.want || got.AddressCount != wantCount || got.Sentinel != "inherited environment" || got.Stdin != string(stdin) || !reflect.DeepEqual(got.Args, args) {
					t.Fatalf("unexpected subprocess input: %+v", got)
				}
			}
			if got, set := os.LookupEnv("MULTIPASS_SERVER_ADDRESS"); got != tc.inherited || set == tc.unset {
				t.Fatalf("parent environment changed: %q, set=%t", got, set)
			}
		})
	}
}

func TestClientServerAddressConcurrentClients(t *testing.T) {
	t.Setenv("MULTIPASS_TEST_ECHO_ENV", "1")
	t.Setenv("MULTIPASS_SERVER_ADDRESS", "inherited.example:50051")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, address := range []string{"first.example:50051", "second.example:50051", ""} {
		cli, err := NewClient(context.Background(), Config{BinaryPath: binary, ServerAddress: address, Timeout: 10})
		if err != nil {
			t.Fatal(err)
		}
		want := address
		if want == "" {
			want = "inherited.example:50051"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				out, err := cli.(*client).run(context.Background(), "list", "--format", "json")
				if err != nil {
					t.Error(err)
					return
				}
				var got commandEnvironment
				if err := json.Unmarshal(out, &got); err != nil {
					t.Error(err)
					return
				}
				if got.Address != want || got.AddressCount != 1 {
					t.Errorf("subprocess address = %q (%d entries), want %q", got.Address, got.AddressCount, want)
				}
			}
		}()
	}
	wg.Wait()
	if got := os.Getenv("MULTIPASS_SERVER_ADDRESS"); got != "inherited.example:50051" {
		t.Fatalf("parent environment changed to %q", got)
	}
}
