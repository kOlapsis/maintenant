package sqlite

// NullableString returns nil if s is empty, otherwise returns s.
// Used for nullable TEXT columns in SQLite.
func NullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
