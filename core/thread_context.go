package core

// applyThreadContext prepends platform-supplied thread/conversation context to
// the prompt when the platform implements ThreadContextProvider and returns a
// non-empty block. This makes the agent aware of context (e.g. earlier Slack
// thread messages) it did not itself receive.
func applyThreadContext(p Platform, replyCtx any, prompt string) string {
	tcp, ok := p.(ThreadContextProvider)
	if !ok {
		return prompt
	}
	ctx := tcp.ThreadContext(replyCtx)
	if ctx == "" {
		return prompt
	}
	return ctx + "\n\n" + prompt
}
