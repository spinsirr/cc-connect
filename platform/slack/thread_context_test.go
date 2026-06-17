package slack

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildThreadTranscript_FormatsPriorMessagesExcludingCurrent(t *testing.T) {
	entries := []threadEntry{
		{who: "alice", text: "we should ship the X connector first", ts: "100.1"},
		{who: "bob", text: "agreed, HaaS depends on it", ts: "100.2"},
		{who: "spencer", text: "@linus what do you think?", ts: "100.3"}, // current message
	}
	got := buildThreadTranscript(entries, "100.3", 30)
	if got == "" {
		t.Fatal("expected a non-empty transcript block")
	}
	if !strings.Contains(got, "alice: we should ship the X connector first") {
		t.Errorf("missing alice line:\n%s", got)
	}
	if !strings.Contains(got, "bob: agreed, HaaS depends on it") {
		t.Errorf("missing bob line:\n%s", got)
	}
	if strings.Contains(got, "what do you think") {
		t.Errorf("current message must be excluded:\n%s", got)
	}
	if strings.Index(got, "alice:") > strings.Index(got, "bob:") {
		t.Errorf("expected oldest-first order:\n%s", got)
	}
}

func TestBuildThreadTranscript_EmptyWhenOnlyCurrentMessage(t *testing.T) {
	entries := []threadEntry{
		{who: "spencer", text: "@linus hi", ts: "100.3"},
	}
	if got := buildThreadTranscript(entries, "100.3", 30); got != "" {
		t.Errorf("expected empty when there is no prior context, got:\n%s", got)
	}
}

func TestBuildThreadTranscript_KeepsMostRecentUpToMax(t *testing.T) {
	entries := []threadEntry{
		{who: "u1", text: "one", ts: "1"},
		{who: "u2", text: "two", ts: "2"},
		{who: "u3", text: "three", ts: "3"},
		{who: "cur", text: "current", ts: "9"},
	}
	got := buildThreadTranscript(entries, "9", 2)
	if strings.Contains(got, "one") {
		t.Errorf("oldest message should be dropped by max=2:\n%s", got)
	}
	if !strings.Contains(got, "two") || !strings.Contains(got, "three") {
		t.Errorf("expected the 2 most recent prior messages:\n%s", got)
	}
}

func TestBuildThreadTranscript_SkipsEmptyAndFlattensNewlines(t *testing.T) {
	entries := []threadEntry{
		{who: "u1", text: "   ", ts: "1"},          // blank → skipped
		{who: "u2", text: "line1\nline2", ts: "2"}, // newline flattened
		{who: "cur", text: "current", ts: "9"},
	}
	got := buildThreadTranscript(entries, "9", 30)
	if strings.Contains(got, "u1:") {
		t.Errorf("blank message should be skipped:\n%s", got)
	}
	if !strings.Contains(got, "u2: line1 line2") {
		t.Errorf("newlines should be flattened to spaces:\n%s", got)
	}
}

func TestThreadContextWith_InjectsOnceThenSkips(t *testing.T) {
	p := &Platform{threadContext: true, threadContextMax: 30}
	rc := replyContext{channel: "C1", timestamp: "100.3"}
	calls := 0
	fetch := func(channel, ts string, limit int) ([]threadEntry, error) {
		calls++
		return []threadEntry{
			{who: "alice", text: "earlier message", ts: "100.1"},
			{who: "spencer", text: "current", ts: "100.3"},
		}, nil
	}
	first := p.threadContextWith(rc, fetch)
	if !strings.Contains(first, "alice: earlier message") {
		t.Fatalf("expected context on first call, got:\n%s", first)
	}
	if second := p.threadContextWith(rc, fetch); second != "" {
		t.Errorf("expected empty on second call (once per thread), got:\n%s", second)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 fetch, got %d", calls)
	}
}

func TestThreadContextWith_RetriesAfterFetchError(t *testing.T) {
	p := &Platform{threadContext: true, threadContextMax: 30}
	rc := replyContext{channel: "C1", timestamp: "200.1"}
	fail := func(channel, ts string, limit int) ([]threadEntry, error) {
		return nil, fmt.Errorf("transient")
	}
	if got := p.threadContextWith(rc, fail); got != "" {
		t.Errorf("expected empty on fetch error, got:\n%s", got)
	}
	ok := func(channel, ts string, limit int) ([]threadEntry, error) {
		return []threadEntry{
			{who: "bob", text: "recovered", ts: "200.0"},
			{who: "me", text: "current", ts: "200.1"},
		}, nil
	}
	if got := p.threadContextWith(rc, ok); !strings.Contains(got, "bob: recovered") {
		t.Errorf("expected retry to inject after a prior fetch error, got:\n%s", got)
	}
}

func TestThreadContextWith_DisabledOrNotInThread(t *testing.T) {
	fetch := func(channel, ts string, limit int) ([]threadEntry, error) {
		t.Fatal("fetch should not be called")
		return nil, nil
	}
	disabled := &Platform{threadContext: false, threadContextMax: 30}
	if got := disabled.threadContextWith(replyContext{channel: "C1", timestamp: "1"}, fetch); got != "" {
		t.Errorf("disabled should return empty, got:\n%s", got)
	}
	enabled := &Platform{threadContext: true, threadContextMax: 30}
	if got := enabled.threadContextWith(replyContext{channel: "C1", timestamp: ""}, fetch); got != "" {
		t.Errorf("no thread_ts should return empty, got:\n%s", got)
	}
}
