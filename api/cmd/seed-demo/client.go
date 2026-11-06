package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"
)

type H = map[string]any

// pace stays under the API's own limits, which also apply here.
const pace = 21 * time.Millisecond

// call sends one request through the real router. It retries when rate limited.
func (s *sample) call(person, method, path string, body any, headers ...string) (int, H) {
	raw, _ := json.Marshal(body)
	for try := 0; ; try++ {
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+person)
		req.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		w := httptest.NewRecorder()
		s.engine.ServeHTTP(w, req)
		s.calls++
		time.Sleep(pace)
		if w.Code == http.StatusTooManyRequests && try < 8 {
			time.Sleep(2 * time.Second)
			continue
		}
		out := H{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
}

// must fails the seed on anything but a success, because a sample that breaks a rule
// is a wrong sample.
func (s *sample) must(person, method, path string, body any, headers ...string) H {
	code, out := s.call(person, method, path, body, headers...)
	if code >= 300 {
		panic(fmt.Sprintf("%s %s as %s: %d %v (sent %v)", method, path, person, code, out, body))
	}
	if d, ok := out["data"].(map[string]any); ok {
		return d
	}
	return out
}

// create adds a record and remembers it under key.
func (s *sample) create(person, path, key string, body H) string {
	id, _ := s.must(person, "POST", path, body)["id"].(string)
	if key != "" {
		s.ref[key] = id
	}
	return id
}

func list(out H) []any {
	rows, _ := out["data"].([]any)
	return rows
}
