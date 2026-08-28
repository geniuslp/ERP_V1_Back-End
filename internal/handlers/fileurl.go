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

// toAbsoluteFileURL prefixes a relative stored path with the CURRENT publicBaseURL.
//
// If path is already absolute (e.g. a stale "http://old-host/uploads/..." baked into the DB
// from before PUBLIC_BASE_URL was configured correctly, or from before it existed at all —
// clients echo back whatever URL an upload endpoint returned, and that got stored verbatim),
// the scheme+host is stripped and replaced with the current publicBaseURL rather than passed
// through as-is. Without this, a row that ever captured an absolute URL would be permanently
// stuck on whatever host was live at the time it was saved, immune to any future
// PUBLIC_BASE_URL change (a real incident: rows saved while PUBLIC_BASE_URL defaulted to
// http://localhost:8080 kept returning that host forever, even after PUBLIC_BASE_URL was set
// correctly, because the old pass-through logic never re-derived the host on read).
func toAbsoluteFileURL(path string) string {
	if path == "" {
		return path
	}
	path = stripKnownHost(path)
	return publicBaseURL + "/" + strings.TrimLeft(path, "/")
}

// stripKnownHost removes a "scheme://host[:port]" prefix from an absolute URL, returning just
// the path portion (no leading slash). Non-absolute input passes through unchanged.
func stripKnownHost(path string) string {
	idx := strings.Index(path, "://")
	if idx == -1 {
		return path
	}
	rest := path[idx+3:]
	if slash := strings.Index(rest, "/"); slash != -1 {
		return rest[slash+1:]
	}
	return "" // absolute URL with no path component
}

func toAbsoluteFileURLPtr(path *string) *string {
	if path == nil {
		return nil
	}
	v := toAbsoluteFileURL(*path)
	return &v
}

// toRelativeDiskPath reverses toAbsoluteFileURL: given a value that may be an absolute URL
// under the current or a stale/prior publicBaseURL (as returned by the upload endpoints, and
// thus as clients may send it back when referencing a pre-uploaded file) or an already-relative
// disk path, returns the relative disk path suitable for os.Stat/os.Open. Strips ANY
// "scheme://host[:port]" prefix, not just the current publicBaseURL, so a file_path saved under
// an old host still resolves to a real on-disk path (see toAbsoluteFileURL for why the host
// can't be trusted to match the current config).
func toRelativeDiskPath(path string) string {
	return stripKnownHost(path)
}
