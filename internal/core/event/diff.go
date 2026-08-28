package event

// Diff keeps only the entries whose before differs from after. Publishers use it to
// decide whether a *FieldsChanged event is worth emitting at all.
//
// It replaces activity.DiffFields, which returned the journal's own wire shape
// ({field: {"before": x, "after": y}}). Producing that shape is the journal's job
// now (see service/activity/journal.go), not the publisher's.
func Diff(pairs map[string][2]any) map[string][2]any {
	out := make(map[string][2]any, len(pairs))
	for field, ba := range pairs {
		if ba[0] != ba[1] {
			out[field] = ba
		}
	}
	return out
}
