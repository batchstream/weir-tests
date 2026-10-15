package fixture

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestSearchReadinessUsesFixtureStartupDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const startupTimeout = 8 * time.Minute
		started := time.Now()
		ctx, cancel := context.WithTimeout(t.Context(), startupTimeout)
		defer cancel()
		cluster := &Cluster{SearchURL: "://invalid"}
		err := cluster.readySearch(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("Search readiness did not preserve the startup deadline", err)
		}
		if elapsed := time.Since(started); elapsed != startupTimeout {
			t.Fatalf("Search readiness stopped after %s, want parent startup deadline %s", elapsed, startupTimeout)
		}
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatal("Search readiness stopped before the parent startup context expired", ctx.Err())
		}
	})
}
