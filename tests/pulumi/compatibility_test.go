package pulumi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const bridge = "terraform-provider@1.4.0"

// This opt-in test uses the real bridge and language runtimes, but a CLI fixture
// that only implements reads. It needs neither Multipass nor a Pulumi account.
func TestPulumiCompatibility(t *testing.T) {
	if os.Getenv("PULUMI_TEST") != "1" {
		t.Skip("set PULUMI_TEST=1 to run Pulumi schema, SDK, and preview checks")
	}
	for _, tool := range []string{"go", "pulumi"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s is required: %v", tool, err)
		}
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	cleanupWindowsPlugins(t, work)
	backend := filepath.Join(work, "state")
	if err := os.MkdirAll(backend, 0700); err != nil {
		t.Fatal(err)
	}
	backendPath := filepath.ToSlash(backend)
	if !strings.HasPrefix(backendPath, "/") {
		backendPath = "/" + backendPath
	}
	t.Setenv("PULUMI_HOME", filepath.Join(work, "pulumi-home"))
	t.Setenv("PULUMI_BACKEND_URL", (&url.URL{Scheme: "file", Path: backendPath}).String())
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "pulumi-compatibility-test")
	t.Setenv("PULUMI_ACCESS_TOKEN", "")
	t.Setenv("PULUMI_SKIP_UPDATE_CHECK", "true")

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	run(t, repo, "go", "build", "-o", filepath.Join(work, "bin", "terraform-provider-multipass"+ext), "./cmd/terraform-provider-multipass")
	fakeCLI := filepath.Join(work, "bin", "fake-multipass"+ext)
	run(t, repo, "go", "build", "-o", fakeCLI, "./tests/pulumi/testdata/fake-multipass")
	run(t, work, "pulumi", "plugin", "install", "resource", "terraform-provider", "1.4.0", "--non-interactive")

	t.Run("schema", func(t *testing.T) {
		// Keep the argument extensionless on Windows: bridge 1.4.0 derives the
		// package name from the filename, and Go resolves the .exe for execution.
		data := run(t, work, "pulumi", "package", "get-schema", bridge, "--", "./bin/terraform-provider-multipass")
		checkSchema(t, data)
	})

	for _, language := range []string{"typescript", "python"} {
		t.Run(language, func(t *testing.T) {
			project := filepath.Join(work, language)
			if err := os.MkdirAll(project, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PULUMI_TEST_COMMAND_LOG", filepath.Join(project, "commands.log"))
			files := []string{"Pulumi.yaml", "__main__.py", "requirements.txt"}
			if language == "typescript" {
				files = []string{"Pulumi.yaml", "index.ts", "package.json", "tsconfig.json"}
			}
			for _, name := range files {
				data, err := os.ReadFile(filepath.Join(repo, "examples", "pulumi", language, name))
				if err != nil {
					t.Fatal(err)
				}
				if name == "Pulumi.yaml" {
					manifest := strings.ReplaceAll(string(data), "\r\n", "\n")
					const published = "      - registry.terraform.io/todoroff/multipass\n      - 2.0.0"
					if !strings.Contains(manifest, published) {
						t.Fatal("update the test's provider pin to match the example manifest")
					}
					data = []byte(strings.Replace(manifest, published, "      - ./../bin/terraform-provider-multipass", 1))
				}
				if err := os.WriteFile(filepath.Join(project, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}

			run(t, project, "pulumi", "install", "--non-interactive")
			if language == "typescript" {
				run(t, project, "node", "node_modules/typescript/bin/tsc", "--noEmit")
			}
			run(t, project, "pulumi", "stack", "init", "smoke", "--non-interactive")
			run(t, project, "pulumi", "config", "set", "multipassPath", fakeCLI, "--plaintext", "--non-interactive")
			// Exercise non-empty nested blocks without requiring host NICs or mounts.
			run(t, project, "pulumi", "config", "set", "hostPath", work, "--plaintext", "--non-interactive")
			run(t, project, "pulumi", "config", "set", "network", "test-network", "--plaintext", "--non-interactive")
			data := run(t, project, "pulumi", "preview", "--json", "--non-interactive")
			var preview struct {
				Steps []struct {
					Op  string `json:"op"`
					URN string `json:"urn"`
				} `json:"steps"`
			}
			if err := json.Unmarshal(data, &preview); err != nil {
				t.Fatalf("decode preview: %v\n%s", err, data)
			}
			for _, token := range []string{"multipass:index/instance:Instance", "multipass:index/alias:Alias", "multipass:index/fileUpload:FileUpload"} {
				found := false
				for _, step := range preview.Steps {
					if strings.Contains(step.URN, "::"+token+"::") && step.Op == "create" {
						found = true
					}
				}
				if !found {
					t.Errorf("preview did not plan creation of %s\n%s", token, data)
				}
			}
			checkCommands(t)
		})
	}
}

func checkCommands(t *testing.T) {
	t.Helper()
	log, err := os.ReadFile(os.Getenv("PULUMI_TEST_COMMAND_LOG"))
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"version --format json", "find --format json"} {
		if !strings.Contains(string(log), command+"\n") {
			t.Errorf("previews did not exercise CLI command %q", command)
		}
	}
	for _, command := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		if command != "version --format json" && command != "find --format json" {
			t.Errorf("preview attempted unexpected CLI command %q", command)
		}
	}
}

// The bridge can leave plugin children alive on Windows after CLI exit. Stop
// only executables inside this test's unique directory before TempDir removes
// it; otherwise their locked binaries cause cleanup to fail.
func cleanupWindowsPlugins(t *testing.T, work string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `
$ErrorActionPreference = 'Stop'
$prefix = $env:PULUMI_TEST_WORK_DIR + [IO.Path]::DirectorySeparatorChar
Get-CimInstance Win32_Process | Where-Object {
    $_.ExecutablePath -and $_.ExecutablePath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)
} | ForEach-Object {
    Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
    Wait-Process -Id $_.ProcessId -ErrorAction SilentlyContinue
}`)
		cmd.Env = append(os.Environ(), "PULUMI_TEST_WORK_DIR="+work)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("clean up Windows test plugins: %v\n%s", err, output)
		}
	})
}

type schemaProperty struct {
	Type   string `json:"type"`
	Ref    string `json:"$ref"`
	Secret bool   `json:"secret"`
}

type schemaResource struct {
	Inputs     map[string]schemaProperty `json:"inputProperties"`
	Properties map[string]schemaProperty `json:"properties"`
}

func checkSchema(t *testing.T, data []byte) {
	t.Helper()
	var schema struct {
		Name      string                     `json:"name"`
		Resources map[string]schemaResource  `json:"resources"`
		Functions map[string]json.RawMessage `json:"functions"`
		Provider  schemaResource             `json:"provider"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Name != "multipass" {
		t.Errorf("package name = %q, want multipass", schema.Name)
	}
	for _, resource := range []string{"alias:Alias", "fileDownload:FileDownload", "fileUpload:FileUpload", "instance:Instance", "snapshot:Snapshot"} {
		if _, ok := schema.Resources["multipass:index/"+resource]; !ok {
			t.Errorf("missing bridged resource %s", resource)
		}
	}
	for _, function := range []string{"getImages", "getInstance", "getNetworks", "getSnapshots"} {
		if _, ok := schema.Functions["multipass:index/"+function+":"+function]; !ok {
			t.Errorf("missing bridged data source %s", function)
		}
	}
	for name, wantType := range map[string]string{"multipassPath": "string", "commandTimeout": "number", "defaultImage": "string"} {
		if schema.Provider.Inputs[name].Type != wantType {
			t.Errorf("provider input %s must have type %s", name, wantType)
		}
	}
	instance := schema.Resources["multipass:index/instance:Instance"]
	for name, wantType := range map[string]string{"cpus": "number", "memory": "string", "disk": "string", "resizePolicy": "string", "networks": "array", "mounts": "array"} {
		if instance.Inputs[name].Type != wantType {
			t.Errorf("instance input %s must have type %s", name, wantType)
		}
	}
	if instance.Inputs["timeouts"].Ref != "#/types/multipass:index/InstanceTimeouts:InstanceTimeouts" {
		t.Error("instance timeouts must bridge as a single object")
	}
	if instance.Properties["ipv4s"].Type != "array" {
		t.Error("instance addresses must be exposed as ipv4s")
	}
	upload := schema.Resources["multipass:index/fileUpload:FileUpload"]
	if !instance.Inputs["cloudInit"].Secret || !instance.Properties["cloudInit"].Secret ||
		!upload.Inputs["content"].Secret || !upload.Properties["content"].Secret {
		t.Error("cloud-init and upload content must retain secret annotations on inputs and outputs")
	}
}

func run(t *testing.T, dir, executable string, args ...string) []byte {
	t.Helper()
	t.Logf("%s %s", executable, strings.Join(args, " "))
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s\n%s", executable, err, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}
