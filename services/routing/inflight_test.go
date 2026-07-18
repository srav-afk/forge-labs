package routing

import "testing"

func TestInflightTrackNoLeak(t *testing.T) {
	tr := NewInflightTracker()
	done := tr.Track("w1")
	if tr.Get("w1") != 1 {
		t.Fatalf("got %d", tr.Get("w1"))
	}
	done()
	done() // second call no-op
	if tr.Get("w1") != 0 {
		t.Fatalf("leaked %d", tr.Get("w1"))
	}
	tr.Dec("w1")
	if tr.Get("w1") != 0 {
		t.Fatalf("underflow %d", tr.Get("w1"))
	}
}
