package slack

import (
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
)

const (
	// threadContextHeader labels the injected transcript so the agent treats it
	// as background, not as messages to answer.
	threadContextHeader = "[Thread context — earlier messages in this Slack thread, oldest first. Reply to the newest message below; use these only as background, do not respond to them directly.]"

	// threadEntryMaxBody caps each rendered message body (in runes) to keep the
	// injected block bounded.
	threadEntryMaxBody = 500

	// defaultThreadContextMax is the fallback cap on how many prior messages to
	// include when the platform option is unset or non-positive.
	defaultThreadContextMax = 30
)

// threadEntry is a single resolved message in a Slack thread, used to build the
// human-readable transcript that cc-connect injects into the agent prompt so
// the agent is aware of thread context it did not itself receive.
type threadEntry struct {
	who  string // resolved display name, falling back to user id / bot id
	text string
	ts   string // slack message timestamp, e.g. "1700000000.000100"
}

// buildThreadTranscript renders recent thread messages as a labelled context
// block. It excludes the current message (ts == currentTS), skips blank
// messages, flattens whitespace, truncates long bodies, and keeps at most
// maxMessages (the most recent), oldest-first. Returns "" when there is no
// prior context worth injecting.
func buildThreadTranscript(entries []threadEntry, currentTS string, maxMessages int) string {
	if maxMessages <= 0 {
		maxMessages = defaultThreadContextMax
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ts == currentTS {
			continue
		}
		// strings.Fields collapses all runs of whitespace (incl. newlines) to
		// single spaces and trims the ends.
		body := strings.Join(strings.Fields(e.text), " ")
		if body == "" {
			continue
		}
		if r := []rune(body); len(r) > threadEntryMaxBody {
			body = string(r[:threadEntryMaxBody]) + "…"
		}
		who := e.who
		if who == "" {
			who = "unknown"
		}
		lines = append(lines, who+": "+body)
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > maxMessages {
		lines = lines[len(lines)-maxMessages:]
	}
	return threadContextHeader + "\n" + strings.Join(lines, "\n")
}

// threadFetcher pulls a thread's recent messages as resolved entries. Injected
// so the gate/orchestration in threadContextWith is testable without a live
// Slack client.
type threadFetcher func(channel, threadTS string, limit int) ([]threadEntry, error)

// ThreadContext implements core.ThreadContextProvider. When the inbound message
// is part of a thread, it fetches the thread's recent replies and returns a
// labelled transcript (excluding the current message), at most once per thread
// per process. Fails open (returns "") when disabled, not in a thread, or on any
// API error. NOTE: after an idle session reset the agent loses memory but the
// thread stays marked seen; a process restart re-arms injection.
func (p *Platform) ThreadContext(rctx any) string {
	rc, ok := rctx.(replyContext)
	if !ok {
		return ""
	}
	return p.threadContextWith(rc, p.fetchThreadEntries)
}

// threadContextWith runs the enabled/in-thread checks, the once-per-thread gate,
// the fetch (via the injected fetcher), and the transcript build. On fetch error
// it releases the gate reservation so the next turn can retry a transient failure.
func (p *Platform) threadContextWith(rc replyContext, fetch threadFetcher) string {
	if !p.threadContext {
		return ""
	}
	if rc.channel == "" || rc.timestamp == "" {
		return ""
	}
	limit := p.threadContextMax
	if limit <= 0 {
		limit = defaultThreadContextMax
	}
	key := rc.channel + ":" + rc.timestamp
	// Reserve up front so concurrent turns in the same thread don't double-fetch.
	if _, seen := p.threadContextSeen.LoadOrStore(key, true); seen {
		return ""
	}
	entries, err := fetch(rc.channel, rc.timestamp, limit)
	if err != nil {
		p.threadContextSeen.Delete(key) // allow retry on transient failure
		slog.Warn("slack: thread context fetch failed", "channel", rc.channel, "thread_ts", rc.timestamp, "error", err)
		return ""
	}
	return buildThreadTranscript(entries, rc.timestamp, limit)
}

// fetchThreadEntries pulls a thread's replies via conversations.replies and
// resolves each author's display name (falling back to bot id).
func (p *Platform) fetchThreadEntries(channel, threadTS string, limit int) ([]threadEntry, error) {
	msgs, _, _, err := p.client.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: channel,
		Timestamp: threadTS,
		Limit:     limit + 5, // headroom for the current message and skipped blanks
	})
	if err != nil {
		return nil, err
	}
	entries := make([]threadEntry, 0, len(msgs))
	for _, m := range msgs {
		who := p.resolveUserName(m.User)
		if who == "" {
			who = m.BotID
		}
		entries = append(entries, threadEntry{who: who, text: m.Text, ts: m.Timestamp})
	}
	return entries, nil
}
