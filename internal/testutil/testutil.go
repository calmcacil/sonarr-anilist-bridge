// Package testutil provides shared test helpers used across internal packages.
package testutil

// Ptr returns a pointer to a copy of v. Handy for constructing struct
// literals with pointer fields in tests.
func Ptr[T any](v T) *T {
	return &v
}
