package clipsync

import "testing"

func TestRemoteEchoIsSuppressedButLaterCopyIsSent(t *testing.T) {
	var tracker Tracker

	tracker.AppliedRemote("A")
	if tracker.ShouldSend("A") {
		t.Fatal("remote clipboard value echoed back")
	}
	if !tracker.ShouldSend("B") {
		t.Fatal("new local clipboard value was suppressed")
	}
	if !tracker.ShouldSend("A") {
		t.Fatal("later intentional copy of the old remote value was suppressed")
	}
}

func TestUnchangedLocalClipboardIsSentOnce(t *testing.T) {
	var tracker Tracker

	if !tracker.ShouldSend("local") {
		t.Fatal("new local clipboard was not sent")
	}
	if tracker.ShouldSend("local") {
		t.Fatal("unchanged local clipboard was sent twice")
	}
}
