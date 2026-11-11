package oplog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendRead(t *testing.T) {
	l := Log{Dir: filepath.Join(t.TempDir(), "state", "zap")}
	got, skipped, err := l.Read()
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, skipped)

	t0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	require.NoError(t, l.Append(Entry{Run: "r1", Time: t0, Path: "/a", Result: "zapped"}))
	require.NoError(t, l.Append(Entry{Run: "r2", Time: t0.Add(time.Hour), Path: "/b", Result: "skipped"}))

	// A torn write must not hide the rest of the history.
	f, err := os.OpenFile(filepath.Join(l.Dir, FileName), os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, _ = f.WriteString("{\"run\":\"r3\",\"pa")
	require.NoError(t, f.Close())

	got, skipped, err = l.Read()
	require.NoError(t, err)
	assert.Equal(t, 1, skipped)
	require.Len(t, got, 2)
	assert.Equal(t, "/b", got[0].Path, "newest first")
	assert.True(t, got[1].Time.Equal(t0))
}
