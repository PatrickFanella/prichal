package orgaccess

import "net/url"

// listQuery accepts one named continuation and bounds bytes before parsing.
// The parameter name and cursor limit are fixed by each route, never user-selected.
func listQuery(raw, parameter string, limit int) (string, error) {
	if len(raw) > limit+128 {
		return "", ErrCursor
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) > 1 {
		return "", ErrCursor
	}
	for key, value := range values {
		if key != parameter || len(value) != 1 || value[0] == "" || len(value[0]) > limit {
			return "", ErrCursor
		}
	}
	return values.Get(parameter), nil
}
