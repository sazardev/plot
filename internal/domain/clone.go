package domain

// CloneDataset returns a deep-enough copy for transformations that mutate
// rows (filter/sort) and append warnings. Cell values are immutable.
func CloneDataset(ds *Dataset) *Dataset {
	if ds == nil {
		return nil
	}
	out := *ds
	out.Columns = append([]Column(nil), ds.Columns...)
	out.Rows = make([][]Cell, len(ds.Rows))
	for i, row := range ds.Rows {
		out.Rows[i] = append([]Cell(nil), row...)
	}
	out.Warnings = make([]Warning, len(ds.Warnings))
	for i, warning := range ds.Warnings {
		out.Warnings[i] = Warning{ID: warning.ID}
		if warning.Data != nil {
			out.Warnings[i].Data = make(map[string]string, len(warning.Data))
			for key, value := range warning.Data {
				out.Warnings[i].Data[key] = value
			}
		}
	}
	return &out
}
