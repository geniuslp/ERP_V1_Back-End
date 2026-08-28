package handlers

import "strings"

// publicBaseURL is the externally reachable origin (scheme+host[:port]) this API is served
// behind. Set once at startup via SetPublicBaseURL. Relative file paths returned by handlers
// (e.g. "uploads/pr/2026/08/x.pdf") are absolutized against it so a client opening the link
// in a new tab hits this backend's static file route directly, instead of guessing an origin.
var publicBaseURL string

// SetPublicBaseURL configures the base URL used to absolutize file paths. Call once from
// routes.Register at startup.
func SetPublicBaseURL(base string) {
	publicBaseURL = strings.TrimRight(base, "/")
}

// toAbsoluteFileURL prefixes a relative stored path with publicBaseURL. Already-absolute
// URLs and empty paths pass through unchanged.
func toAbsoluteFileURL(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return publicBaseURL + "/" + strings.TrimLeft(path, "/")
}

func toAbsoluteFileURLPtr(path *string) *string {
	if path == nil {
		return nil
	}
	v := toAbsoluteFileURL(*path)
	return &v
}
