package capability

import (
	"fmt"

	arSchema "github.com/permadao/goar/schema"
)

// DefaultMaxBytes bounds the total expanded size of a public.zip payload.
const DefaultMaxBytes = 64 << 20

// CodedError carries a stable error code for Apply(Result.Error).
type CodedError struct {
	Code string
	Err  error
}

func (e *CodedError) Error() string { return e.Code + ": " + e.Err.Error() }

func coded(code, format string, a ...any) error {
	return &CodedError{Code: code, Err: fmt.Errorf(format, a...)}
}

// tagValue returns the value of the named tag, or "" if absent.
func tagValue(tags []arSchema.Tag, name string) string {
	for _, tag := range tags {
		if tag.Name == name {
			return tag.Value
		}
	}
	return ""
}
