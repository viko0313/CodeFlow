package permission

import (
	"context"
	"testing"
)

func TestGateRejectsTraversalBeforeConfirmation(t *testing.T) {
	gate := NewGate(Options{Confirmer: func(context.Context, Operation) (Decision, error) {
		t.Fatal("confirmer should not be called for invalid paths")
		return Decision{}, nil
	}})
	decision, err := gate.Review(context.Background(), Operation{Kind: OperationWriteFile, ProjectRoot: t.TempDir(), Path: "../x"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestGateTrustedShellCommandSkipsConfirmation(t *testing.T) {
	gate := NewGate(Options{
		TrustedCommands: []string{"go test"},
		Confirmer: func(context.Context, Operation) (Decision, error) {
			t.Fatal("confirmer should not be called for trusted command")
			return Decision{}, nil
		},
	})
	decision, err := gate.Review(context.Background(), Operation{Kind: OperationShell, ProjectRoot: t.TempDir(), Command: "go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Reason != "trusted command" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestShellValidationBlocksDangerousCommands(t *testing.T) {
	if err := ValidateShellCommand("python -c \"print(1)\""); err == nil {
		t.Fatal("expected python -c to be blocked")
	}
}
