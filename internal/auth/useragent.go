package auth

import "strings"

// UserAgentInfo is a coarse human-readable breakdown of a User-Agent string.
// Either field may be empty when nothing recognisable was found.
type UserAgentInfo struct {
	Browser string
	OS      string
}

// DisplayName joins Browser and OS the way the UI shows them ("Chrome on macOS").
// Falls back to whichever single field is set, or "Unknown" if both are empty.
func (u UserAgentInfo) DisplayName() string {
	switch {
	case u.Browser != "" && u.OS != "":
		return u.Browser + " on " + u.OS
	case u.Browser != "":
		return u.Browser
	case u.OS != "":
		return u.OS
	default:
		return "Unknown"
	}
}

// ParseUserAgent extracts a coarse browser/OS pair from raw UA. It recognises
// the major desktop/mobile browsers plus the custom UA shapes used by
// Turboist clients (e.g. "Turboist-iOS/1.2 (iPhone; iOS 17.2)" or
// "turboist-cli/0.4 (macOS)"). Anything else falls back to empty fields.
func ParseUserAgent(raw string) UserAgentInfo {
	ua := strings.TrimSpace(raw)
	if ua == "" {
		return UserAgentInfo{}
	}

	if info, ok := parseTurboistClient(ua); ok {
		return info
	}

	return UserAgentInfo{
		Browser: detectBrowser(ua),
		OS:      detectOS(ua),
	}
}

// parseTurboistClient handles UA strings produced by first-party clients —
// kept here so iOS/Android/CLI apps get a clean label without polluting the
// generic browser/OS heuristics below.
func parseTurboistClient(ua string) (UserAgentInfo, bool) {
	lower := strings.ToLower(ua)
	switch {
	case strings.HasPrefix(lower, "turboist-ios"):
		return UserAgentInfo{Browser: "Turboist iOS", OS: detectOS(ua)}, true
	case strings.HasPrefix(lower, "turboist-android"):
		return UserAgentInfo{Browser: "Turboist Android", OS: detectOS(ua)}, true
	case strings.HasPrefix(lower, "turboist-cli"):
		return UserAgentInfo{Browser: "Turboist CLI", OS: detectOS(ua)}, true
	}
	return UserAgentInfo{}, false
}

func detectBrowser(ua string) string {
	// Order matters: Edge/Opera/Chromium-based UAs also contain "Chrome"
	// and "Safari" tokens, so the more specific brand wins first.
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "EdgA/") || strings.Contains(ua, "EdgiOS/"):
		return "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		return "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		return "Firefox"
	case strings.Contains(ua, "CriOS/"):
		return "Chrome"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	case strings.Contains(ua, "Safari/"):
		return "Safari"
	}
	return ""
}

func detectOS(ua string) string {
	switch {
	case strings.Contains(ua, "Windows NT"):
		return "Windows"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") || strings.Contains(ua, "iPod"):
		return "iOS"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "macOS"):
		return "macOS"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	}
	return ""
}
