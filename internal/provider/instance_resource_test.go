package provider

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func TestInstanceRejectsReadOnlyMountConfig(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(NewInstanceResource())
	typ := s.Type().TerraformType(ctx).(tftypes.Object)
	mountType := typ.AttributeTypes["mounts"].(tftypes.List).ElementType
	for _, readOnly := range []bool{false, true} {
		mount := tftypes.NewValue(mountType, map[string]tftypes.Value{
			"host_path":     tftypes.NewValue(tftypes.String, "/host"),
			"instance_path": tftypes.NewValue(tftypes.String, "/workspace"),
			"read_only":     tftypes.NewValue(tftypes.Bool, readOnly),
		})
		config := resourceValue(s, map[string]any{"name": "vm", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{mount}})
		server := providerserver.NewProtocol6(New("test")())()
		resp, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: "multipass_instance", Config: dynamicValue(t, config)})
		if err != nil {
			t.Fatal(err)
		}
		hasError := false
		for _, d := range resp.Diagnostics {
			hasError = hasError || d.Severity == tfprotov6.DiagnosticSeverityError
		}
		if hasError != readOnly {
			t.Fatalf("read_only=%t diagnostics: %v", readOnly, resp.Diagnostics)
		}
	}
}

func TestInstanceRejectsReadOnlyMountBeforeUpdating(t *testing.T) {
	ctx := context.Background()
	// No mock mutating methods are provided: any attempted unmount or mount
	// panics, proving validation happens before changing existing mounts.
	r := &instanceResource{client: &testClient{}, commandTimeout: time.Second}
	s := resourceSchema(r)
	typ := s.Type().TerraformType(ctx).(tftypes.Object)
	mountType := typ.AttributeTypes["mounts"].(tftypes.List).ElementType
	mount := tftypes.NewValue(mountType, map[string]tftypes.Value{
		"host_path":     tftypes.NewValue(tftypes.String, "/host"),
		"instance_path": tftypes.NewValue(tftypes.String, "/workspace"),
		"read_only":     tftypes.NewValue(tftypes.Bool, true),
	})
	values := map[string]any{"id": "vm", "name": "vm", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	values["mounts"] = []tftypes.Value{mount}
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}, State: state}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read-only mount rejection")
	}
}

func TestInstanceCloudInitResult(t *testing.T) {
	exitErr := cloudInitCLIExit(t, 2)
	degraded := func(stdout string) error {
		return &multipasscli.CLIError{Command: "exec vm -- cloud-init status --format json", Stdout: stdout, Err: exitErr}
	}
	for _, tc := range []struct {
		name         string
		wait         bool
		waitErr      error
		warning      bool
		queryFailure bool
	}{
		{"legacy success without JSON support", true, nil, false, false},
		{"failure", true, errors.New("cloud-init exited 1"), false, false},
		{"timeout", true, context.DeadlineExceeded, false, false},
		{"cancelled", true, context.Canceled, false, false},
		{"disabled", false, nil, false, false},
		{"hostname warning", true, degraded(`{"status":"done","errors":[],"recoverable_errors":{"WARNING":["Failed to set the hostname"]}}`), true, false},
		{"deprecation", true, degraded(`{"status":"done","errors":[],"recoverable_errors":{"DEPRECATED":["Deprecated config"]}}`), true, false},
		{"failed module", true, degraded(`{"status":"done","errors":["scripts-user failed"],"recoverable_errors":{"WARNING":["Failed to set the hostname"]}}`), false, false},
		{"recoverable error", true, degraded(`{"status":"done","errors":[],"recoverable_errors":{"ERROR":["Failed config"],"WARNING":["Failed to set the hostname"]}}`), false, false},
		{"critical error", true, degraded(`{"status":"done","errors":[],"recoverable_errors":{"CRITICAL":["Failed config"]}}`), false, false},
		{"unknown severity", true, degraded(`{"status":"done","errors":[],"recoverable_errors":{"UNKNOWN":["Unknown problem"]}}`), false, false},
		{"running", true, degraded(`{"status":"running","errors":[],"recoverable_errors":{"WARNING":["Still running"]}}`), false, false},
		{"missing errors", true, degraded(`{"status":"done","recoverable_errors":{"WARNING":["Unknown result"]}}`), false, false},
		{"null errors", true, degraded(`{"status":"done","errors":null,"recoverable_errors":{"WARNING":["Unknown result"]}}`), false, false},
		{"missing warnings", true, degraded(`{"status":"done","errors":[]}`), false, false},
		{"invalid json", true, degraded(`not json`), false, false},
		{"empty output", true, degraded(``), false, false},
		{"status query timeout", true, context.DeadlineExceeded, false, true},
		{"status query cancelled", true, context.Canceled, false, true},
		{"status query failure", true, errors.New("status query unavailable"), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			waitCalls, statusCalls := 0, 0
			var waitExit *exec.ExitError
			queryExpected := tc.queryFailure || errors.As(tc.waitErr, &waitExit)
			r := &instanceResource{commandTimeout: time.Second, client: &testClient{
				launchInstance: func(context.Context, models.LaunchOptions) error { return nil },
				execCapture: func(_ context.Context, _ string, command []string) ([]byte, error) {
					if reflect.DeepEqual(command, []string{"cloud-init", "status", "--wait"}) {
						waitCalls++
						if queryExpected {
							return nil, &multipasscli.CLIError{Command: "exec vm -- cloud-init status --wait", Stdout: "status: done", Err: exitErr}
						}
						return nil, tc.waitErr
					}
					statusCalls++
					if !queryExpected {
						t.Fatal("JSON status must not be requested after a successful or failed legacy wait")
					}
					if !reflect.DeepEqual(command, []string{"cloud-init", "status", "--format", "json"}) {
						t.Fatalf("unexpected status command: %v", command)
					}
					return nil, tc.waitErr
				},
			}}
			s := resourceSchema(r)
			plan := tfsdk.Plan{Schema: s, Raw: resourceValue(s, map[string]any{"name": "vm", "wait_for_cloud_init": tc.wait, "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}})}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: s}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() != (tc.waitErr != nil && !tc.warning) {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			foundWarning := false
			for _, d := range resp.Diagnostics {
				if d.Summary() == "cloud-init completed with warnings" {
					foundWarning = true
					if !strings.Contains(d.Detail(), "Failed to set the hostname") && !strings.Contains(d.Detail(), "Deprecated config") {
						t.Fatalf("missing guest warning: %s", d.Detail())
					}
				}
			}
			if foundWarning != tc.warning {
				t.Fatalf("warning=%t, want %t: %v", foundWarning, tc.warning, resp.Diagnostics)
			}
			var state instanceResourceModel
			if d := resp.State.Get(ctx, &state); d.HasError() {
				t.Fatal(d)
			}
			if state.ID.ValueString() != "vm" {
				t.Fatal("created VM must remain in state even when cloud-init fails")
			}
			if (waitCalls == 1) != tc.wait {
				t.Fatalf("wait calls=%d", waitCalls)
			}
			if (statusCalls == 1) != queryExpected {
				t.Fatalf("status calls=%d, expected query=%t", statusCalls, queryExpected)
			}
		})
	}
}

func TestInstanceReplacementPlan(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(NewInstanceResource())
	for _, tc := range []struct {
		name    string
		field   string
		value   any
		replace bool
	}{
		{"name", "name", "renamed-vm", true},
		{"image", "image", "24.04", true},
		{"reset_image", "image", nil, true},
		{"unchanged", "image", "22.04", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]any{"id": "vm", "name": "vm", "image": "22.04", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
			prior := resourceValue(s, values)
			values[tc.field] = tc.value
			next := resourceValue(s, values)
			delete(values, "id")
			server := providerserver.NewProtocol6(New("test")())()
			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_instance", PriorState: dynamicValue(t, prior), ProposedNewState: dynamicValue(t, next), Config: dynamicValue(t, resourceValue(s, values))})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatalf("%s: %s", d.Summary, d.Detail)
				}
			}
			if (len(resp.RequiresReplace) > 0) != tc.replace {
				t.Fatalf("replacement paths: %v, want replacement=%t", resp.RequiresReplace, tc.replace)
			}
		})
	}
}

func TestEmptyInstanceNameRejectedBeforeLaunch(t *testing.T) {
	ctx := context.Background()
	r := &instanceResource{client: &testClient{}, commandTimeout: time.Second}
	s := resourceSchema(r)
	for _, name := range []any{"", tftypes.UnknownValue} {
		values := map[string]any{"name": name, "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
		if name == "" {
			server := providerserver.NewProtocol6(New("test")())()
			v, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: "multipass_instance", Config: dynamicValue(t, resourceValue(s, values))})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range v.Diagnostics {
				found = found || d.Severity == tfprotov6.DiagnosticSeverityError
			}
			if !found {
				t.Fatal("empty name accepted")
			}
		}
		resp := resource.CreateResponse{State: tfsdk.State{Schema: s}}
		r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("invalid name reached launch")
		}
	}
}

func TestLaunchFailureOwnership(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		before, after         bool
		listErr, launchErr    error
		wantLaunch, wantState bool
	}{
		{name: "mount fails after creation", after: true, launchErr: errors.New("mount failed"), wantLaunch: true, wantState: true},
		{name: "launch fails before creation", launchErr: errors.New("invalid image"), wantLaunch: true},
		{name: "existing VM", before: true},
		{name: "preflight unavailable", listErr: errors.New("daemon unavailable")},
		{name: "concurrent name collision", after: true, launchErr: multipasscli.ErrAlreadyExists, wantLaunch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			launched := false
			r := &instanceResource{commandTimeout: time.Second, client: &testClient{
				listInstances: func(context.Context, bool) ([]models.Instance, error) {
					if tc.listErr != nil {
						return nil, tc.listErr
					}
					if (!launched && tc.before) || (launched && tc.after) {
						return []models.Instance{{Name: "vm", State: "Running"}}, nil
					}
					return nil, nil
				},
				launchInstance: func(context.Context, models.LaunchOptions) error { launched = true; return tc.launchErr },
				execCapture: func(context.Context, string, []string) ([]byte, error) {
					t.Fatal("cloud-init must not run after launch failure")
					return nil, nil
				},
			}}
			s := resourceSchema(r)
			resp := resource.CreateResponse{State: tfsdk.State{Schema: s}}
			plan := tfsdk.Plan{Schema: s, Raw: resourceValue(s, map[string]any{"name": "vm", "wait_for_cloud_init": true, "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}})}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
			if !resp.Diagnostics.HasError() || launched != tc.wantLaunch || !resp.State.Raw.IsNull() != tc.wantState {
				t.Fatalf("launched=%v state=%v diagnostics=%v", launched, resp.State.Raw, resp.Diagnostics)
			}
			if tc.wantState {
				var state instanceResourceModel
				if d := resp.State.Get(context.Background(), &state); d.HasError() {
					t.Fatal(d)
				}
				if state.ID.ValueString() != "vm" {
					t.Fatal("failed VM was not recorded")
				}
				if !strings.Contains(resp.Diagnostics[0].Detail(), tc.launchErr.Error()) {
					t.Fatal("original launch failure lost")
				}
			}
		})
	}
}
