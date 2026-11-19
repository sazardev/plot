package storage_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/storage"
)

func BenchmarkParseInfer10k(b *testing.B) {
	var data bytes.Buffer
	data.WriteString("day,a,b,c,d\n")
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&data, "2024-01-%02d,%d,%d,%d,%d\n", (i%28)+1, i%100, i%50, i%10, i%5)
	}
	payload := data.Bytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ds, err := storage.ReadCSV(bytes.NewReader(payload), storage.CSVOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if err := domain.Infer(ds); err != nil {
			b.Fatal(err)
		}
	}
}
