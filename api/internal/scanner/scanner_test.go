package scanner

import (
	"testing"
	"time"
)

func TestDueFull(t *testing.T) {
	s := New(nil)
	seg := segment{id: "a"}
	if !s.dueFull(seg, "10.0.0.5", false) {
		t.Fatal("a new host must be identified in full")
	}
	if s.dueFull(seg, "10.0.0.5", false) {
		t.Fatal("a host identified moments ago must not be again on a scheduled scan")
	}
	if !s.dueFull(seg, "10.0.0.5", true) {
		t.Fatal("a manual scan identifies every host in full")
	}
	seg.checkPorts = true
	if !s.dueFull(seg, "10.0.0.6", false) || s.dueFull(seg, "10.0.0.6", false) {
		t.Fatal("ports follow the same schedule once checked")
	}
	s.lastFull["a|10.0.0.7"] = fullMark{at: time.Now()}
	if !s.dueFull(seg, "10.0.0.7", false) {
		t.Fatal("a segment that starts checking ports gets them on its next scan")
	}
	s.lastFull["a|10.0.0.8"] = fullMark{at: time.Now().Add(-fullEvery - time.Minute), ports: true}
	if !s.dueFull(seg, "10.0.0.8", false) {
		t.Fatal("a host is identified in full again after fullEvery")
	}
}
