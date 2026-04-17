package chat

func trimToRunes(s string, max int) string {
	if max < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func clampRoleForDisplay(r Role) Role {
	r.Name = trimToRunes(r.Name, roleNameMaxChars())
	if r.Description != nil {
		d := trimToRunes(*r.Description, roleDescriptionMaxChars())
		r.Description = &d
	}
	return r
}

func clampChatSummaryForDisplay(s ChatSummary) ChatSummary {
	// Keep chat title behavior consistent with existing title normalization.
	s.Title = truncateWithEllipsis(s.Title, maxTitleRunes)
	return s
}

func clampMessageForDisplay(m Message) Message {
	m.SenderName = trimToRunes(m.SenderName, roleNameMaxChars())
	m.Content = trimToRunes(m.Content, userMessageMaxChars())
	if m.Reasoning != nil {
		r := trimToRunes(*m.Reasoning, userMessageMaxChars())
		m.Reasoning = &r
	}
	if m.SenderModel != nil {
		model := trimToRunes(*m.SenderModel, 256)
		m.SenderModel = &model
	}
	return m
}
