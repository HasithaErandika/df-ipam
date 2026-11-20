package links

import (
	"encoding/json"
	"slices"
)

// RedactAudit removes what only commercial viewers may see from one audit
// record's values: the commercial fields of a network link, and the file names
// of its contracts and quotations. Other records come back unchanged. The
// general audit log uses it for people without links:commercial_view.
func RedactAudit(objectType string, raw []byte) []byte {
	if objectType != "network_link" || len(raw) == 0 {
		return raw
	}
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		return nil // not an object: show nothing rather than risk showing money
	}
	for _, k := range commercialKeys {
		delete(v, k)
	}
	if cat, _ := v["category"].(string); slices.Contains(commercialFiles, cat) {
		v["attachment"] = "(hidden)"
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return out
}
