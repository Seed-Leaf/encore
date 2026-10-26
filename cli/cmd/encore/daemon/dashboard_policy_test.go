package daemon

import (
	"context"
	"strings"
	"testing"
)

func TestInvalidDashboardPolicyFailsBeforeListeners(t *testing.T) {
	for _, value := range []string{"", "   ", "[]", "null", `["https://*.example.com"]`} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ENCORE_DEVDASH_REMOTE_ORIGINS", value)
			assertInvalidDashboardPolicy(t)
		})
	}
}

func assertInvalidDashboardPolicy(t *testing.T) {
	t.Helper()
	d := &Daemon{}
	err := func() (err error) {
		defer handleBailout(&err)
		d.init(context.Background())
		return nil
	}()
	if err == nil || !strings.Contains(err.Error(), "ENCORE_DEVDASH_REMOTE_ORIGINS") {
		t.Fatalf("expected actionable policy configuration error, got %v", err)
	}
	if d.Daemon != nil || d.Dash != nil || d.Runtime != nil || d.MCP != nil || d.DBProxy != nil || d.Debug != nil || d.ObjectStorage != nil || d.EncoreDB != nil {
		t.Fatal("invalid remote configuration must not initialize listeners or storage")
	}
}
