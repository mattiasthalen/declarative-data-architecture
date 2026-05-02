// prism/internal/pipeline/run_test.go
package pipeline

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/events"
)

// TestRunRunnerWithFakeBinary uses /bin/sh as a stand-in for `uv` to verify
// the orchestration code parses stdout and propagates errors.
func TestRunRunnerWithFakeBinary(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-uv.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"event":"source.start","source":"x"}'
echo '{"event":"entity.end","entity":"E","rows":3,"load_id":"L1","files":1}'
echo '{"event":"source.end","source":"x","entities":1,"duration_ms":10}'
exit 0
`), 0o755))

	var got []events.Event
	var stderr bytes.Buffer
	err := RunRunner(context.Background(), scriptPath, dir, "/dev/null", []string{"/dev/null"}, dir,
		func(e events.Event) error { got = append(got, e); return nil },
		&stderr,
	)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "source.start", got[0].Event)
}

func TestRunRunnerErrorEvent(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-uv.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"event":"error","entity":"E","kind":"X","message":"boom"}'
exit 0
`), 0o755))
	err := RunRunner(context.Background(), scriptPath, dir, "/dev/null", []string{"/dev/null"}, dir,
		func(events.Event) error { return nil }, &bytes.Buffer{},
	)
	require.Error(t, err)
}
