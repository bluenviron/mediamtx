package api //nolint:revive

import "strings"

func search[T any](items []T, query string, field func(T) string) []T {
	if query == "" {
		return items
	}

	query = strings.ToLower(query)
	ret := make([]T, 0, len(items))

	for _, item := range items {
		if strings.Contains(strings.ToLower(field(item)), query) {
			ret = append(ret, item)
		}
	}

	return ret
}
