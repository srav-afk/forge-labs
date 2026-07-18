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

func TestTryTrackRespectsLimit(t *testing.T) {
	tr := NewInflightTracker()
	d1, ok := tr.TryTrack("w1", 2)
	if !ok {
		t.Fatal("first")
	}
	d2, ok := tr.TryTrack("w1", 2)
	if !ok {
		t.Fatal("second")
	}
	if _, ok := tr.TryTrack("w1", 2); ok {
		t.Fatal("third should fail")
	}
	if tr.Get("w1") != 2 {
		t.Fatalf("got %d", tr.Get("w1"))
	}
	d1()
	d2()
	if tr.Get("w1") != 0 {
		t.Fatalf("leaked %d", tr.Get("w1"))
	}
}
