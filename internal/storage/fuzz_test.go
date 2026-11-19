package storage_test

import (
	"bytes"
	"testing"

	"github.com/sazardev/plot/internal/storage"
)

func FuzzReadCSV(f *testing.F) {
	f.Add([]byte("a,b\n1,2\n"))
	f.Add([]byte("a;b\n1;2\n"))
	f.Add([]byte("a,b\n1\n2,3,4\n"))
	f.Add([]byte("\"unterminated\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = storage.ReadCSV(bytes.NewReader(data), storage.CSVOptions{MaxBytes: 1 << 20})
	})
}
