package sizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisk(t *testing.T) {
	free, total, err := Disk(t.TempDir())
	require.NoError(t, err)
	assert.Positive(t, total)
	assert.LessOrEqual(t, free, total)
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{
		"0": 0, "512": 512, "500MB": 500e6, "1GB": 1e9, "1.5 GB": 1.5e9, "10g": 10e9, "2TB": 2e12, "750 kb": 750e3,
	} {
		got, err := ParseSize(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "GB", "lots", "-1GB", "1XB"} {
		_, err := ParseSize(bad)
		assert.Error(t, err, bad)
	}
}
