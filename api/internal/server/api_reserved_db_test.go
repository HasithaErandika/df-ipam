package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"dfipam/api/internal/auth"
)

// An address held by a reservation can't be registered by hand or by import:
// the hold's owner converts it instead.
func TestReservedAddressesStayHeldAgainstDatabase(t *testing.T) {
	e := newAPI(t)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Lab", "kind": "server", "network": "172.16.40.0/24"}, 200))
	hold := id(e.call("POST", "/reservations", map[string]any{"segmentId": seg, "startIp": "172.16.40.10", "endIp": "172.16.40.12", "reason": "Lab racks"}, 201))

	e.call("POST", "/ip-addresses", map[string]any{"segmentId": seg, "ip": "172.16.40.11", "assignedName": "Sneaky", "status": "assigned"}, 409)
	e.call("POST", "/ip-addresses", map[string]any{"segmentId": seg, "ip": "172.16.40.20", "assignedName": "Outside the hold", "status": "assigned"}, 201)

	got := e.importIPs("ip,status,assignedname\n172.16.40.12,assigned,Imported into hold\n172.16.40.21,assigned,Imported outside\n")
	if got["imported"].(float64) != 1 || got["skipped"].(float64) != 1 {
		t.Fatalf("import should skip the held address only: %v", got)
	}

	e.call("POST", "/reservations/"+hold+"/convert", map[string]any{"ip": "172.16.40.11", "name": "lab-11"}, 200)
}

// importIPs uploads a CSV to the register import and returns its result.
func (e *apiEnv) importIPs(csv string) map[string]any {
	e.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "ips.csv")
	if err != nil {
		e.t.Fatal(err)
	}
	_, _ = part.Write([]byte(csv))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ip-addresses/import", &body)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookiePlain, Value: "test"})
	req.Header.Set(auth.CSRFHeader, "DF-IPAM")
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.RemoteAddr = "10.9.250.1:1000"
	res := httptest.NewRecorder()
	e.router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		e.t.Fatalf("import: got %d: %s", res.Code, res.Body.String())
	}
	out := map[string]any{}
	_ = json.Unmarshal(res.Body.Bytes(), &out)
	return out["data"].(map[string]any)
}
