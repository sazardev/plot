package domain_test

import (
	"testing"

	"github.com/sazar/plot/internal/domain"
)

func FuzzFilter(f *testing.F) {
	f.Add("revenue>=10")
	f.Add("month contains J")
	f.Add("revenue!=null")
	f.Add("revenue<=")
	f.Add("")
	f.Add("missing=1")

	f.Fuzz(func(t *testing.T, expression string) {
		ds := dataset(
			[]string{"month", "revenue"},
			[]string{"Jan", "10"},
			[]string{"Feb", "20"},
			[]string{"Mar", "N/A"},
		)
		_ = domain.Infer(ds)
		_, _ = domain.BuildRenderSpec(ds, domain.BuildOptions{
			Filters: []string{expression},
			Width:   80,
			Height:  24,
		})
	})
}

func FuzzInference(f *testing.F) {
	f.Add("Jan", "10")
	f.Add("2024-01-01", "1.5")
	f.Add("", "N/A")
	f.Add("\xff\xfe", "1e3")

	f.Fuzz(func(t *testing.T, label, value string) {
		ds := dataset(
			[]string{"label", "value"},
			[]string{label, value},
			[]string{"other", "2"},
		)
		_ = domain.Infer(ds)
		_, _ = domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	})
}
