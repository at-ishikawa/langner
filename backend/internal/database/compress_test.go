package database

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzipRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{name: "empty", in: []byte{}},
		{name: "yaml", in: []byte("- expression: break the ice\n  meaning: to start a conversation\n")},
		{name: "repetitive", in: bytes.Repeat([]byte("lose one's temper\n"), 500)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Gunzip(Gzip(tt.in))
			require.NoError(t, err)
			assert.Equal(t, tt.in, out)
		})
	}
	assert.Less(t, len(Gzip(tests[2].in)), len(tests[2].in)/10, "repetitive text must compress")

	_, err := Gunzip([]byte("not gzip"))
	assert.Error(t, err)
}
