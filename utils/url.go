package utils

import "strings"

func SanitiseUrl(url string) string {

	if strings.HasSuffix(url, "/") {
		return strings.TrimSuffix(url, "/")
	}
	return url
}
