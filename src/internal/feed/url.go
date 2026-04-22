package feed

import (
	"net/url"
	"strings"
)

var trackingParams = map[string]struct{}{
	"utm_source": {}, "utm_medium": {}, "utm_campaign": {}, "utm_term": {},
	"utm_content": {}, "utm_id": {}, "utm_name": {}, "utm_reader": {},
	"fbclid": {}, "gclid": {}, "dclid": {}, "gbraid": {}, "wbraid": {},
	"mc_cid": {}, "mc_eid": {}, "yclid": {}, "msclkid": {}, "_hsenc": {},
	"_hsmi": {}, "hsCtaTracking": {}, "ref": {}, "ref_src": {}, "ref_url": {},
	"mkt_tok": {}, "igshid": {}, "si": {}, "spm": {}, "trk": {}, "trkCampaign": {},
}

// NormalizeURL produces a canonical form of a URL for cross-source dedup.
// Empty input yields empty output. Invalid URLs fall back to the trimmed original.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	} else {
		u.Scheme = strings.ToLower(u.Scheme)
	}
	u.Host = strings.ToLower(u.Host)
	u.Host = strings.TrimPrefix(u.Host, "www.")
	u.Fragment = ""
	u.RawFragment = ""

	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if _, drop := trackingParams[strings.ToLower(k)]; drop {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}

	if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimRight(u.Path, "/")
	}

	return u.String()
}
