package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"realtime-chat/backend/internal/ai"
)

const maxSummaryItems = 5
const maxSummaryContextMessages = 200

type summaryItem struct {
	Text            string `json:"text"`
	SourceMessageID string `json:"source_message_id,omitempty"`
}

type channelSummary struct {
	ChannelID        string        `json:"channel_id"`
	GeneratedAt      string        `json:"generated_at"`
	Scope            string        `json:"scope"`
	MessageCount     int           `json:"message_count"`
	UnreadCount      int           `json:"unread_count"`
	Summary          string        `json:"summary"`
	Decisions        []summaryItem `json:"decisions"`
	ActionItems      []summaryItem `json:"action_items"`
	Unresolved       []summaryItem `json:"unresolved"`
	ChatterCount     int           `json:"chatter_count"`
	SourceMessageIDs []string      `json:"source_message_ids"`
}

type summaryWire struct {
	Summary     string          `json:"summary"`
	Decisions   json.RawMessage `json:"decisions"`
	ActionItems json.RawMessage `json:"action_items"`
	Unresolved  json.RawMessage `json:"unresolved"`
	Chatter     *int            `json:"chatter_count"`
}

func (s *server) generateChannelSummary(parent context.Context, userID, channelID string) (channelSummary, error) {
	unreadMessages, unreadCount, err := s.repository.ListUnreadMessageContext(parent, userID, channelID, maxSummaryContextMessages)
	if err != nil {
		return channelSummary{}, err
	}
	messages := unreadMessages
	scope := "unread"
	if len(messages) == 0 {
		messages, err = s.repository.ListAIContextMessages(parent, channelID, 50)
		if err != nil {
			return channelSummary{}, err
		}
		scope = "recent"
	}
	result := channelSummary{
		ChannelID:        channelID,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Scope:            scope,
		UnreadCount:      unreadCount,
		Decisions:        []summaryItem{},
		ActionItems:      []summaryItem{},
		Unresolved:       []summaryItem{},
		SourceMessageIDs: make([]string, 0, len(messages)),
	}
	if len(messages) == 0 {
		result.Summary = "まだ要約できるメッセージがありません。"
		return result, nil
	}

	history := make([]ai.Message, 0, len(messages))
	contextCharacters := 0
	for _, message := range messages {
		if contextCharacters >= maxAIContextCharacters {
			break
		}
		body := truncateRunes(aiContextBody(message), maxAIContextCharacters-contextCharacters)
		if body == "" {
			continue
		}
		result.SourceMessageIDs = append(result.SourceMessageIDs, message.ID)
		contextCharacters += len([]rune(body))
		history = append(history, ai.Message{
			Author: message.Author,
			Body:   body,
		})
	}
	result.MessageCount = len(history)
	if len(history) == 0 {
		result.Summary = "要約できる本文がありません。"
		return result, nil
	}

	key := userID + ":" + channelID
	allowed, err := s.acquireAI(parent, key)
	if err != nil || !allowed {
		return channelSummary{}, invalidInput("要点の作成は少し待ってからもう一度お試しください")
	}
	defer s.releaseAI(key)

	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	scopeLabel := "未読メッセージ"
	if scope == "recent" {
		scopeLabel = "最近のメッセージ"
	}
	prompt := fmt.Sprintf(`[WORK_SUMMARY]
このチームチャットの内容を、あとから短時間で把握できるようにまとめてください。
%sだけを対象に、決まったこと、決まった理由、誰かへの依頼、まだ決まっていないことを拾ってください。
必ずJSONだけを返してください。Markdownのコードブロックや説明文は不要です。
各項目は {"text":"短い日本語の要約", "source_message_id":"元メッセージのmessage_id"} の形式にしてください。
判断できないsource_message_idは空文字にしてください。雑談はchatter_countへ数えます。
形式:
{"summary":"全体の短い要約","decisions":[],"action_items":[],"unresolved":[],"chatter_count":0}
`, scopeLabel)
	finalBody, err := s.aiService.Stream(ctx, history, prompt, func(string) error { return nil })
	if err != nil {
		return channelSummary{}, err
	}

	parsed, ok := parseSummaryWire(finalBody)
	if !ok {
		parsed = heuristicSummary(history)
	}
	result.Summary = strings.TrimSpace(parsed.Summary)
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("%d件の会話を確認しました。", len(history))
	}
	allDecisions := decodeSummaryItems(parsed.Decisions)
	allActionItems := decodeSummaryItems(parsed.ActionItems)
	allUnresolved := decodeSummaryItems(parsed.Unresolved)
	result.Decisions = limitSummaryItemList(allDecisions)
	result.ActionItems = limitSummaryItemList(allActionItems)
	result.Unresolved = limitSummaryItemList(allUnresolved)
	if parsed.Chatter != nil {
		result.ChatterCount = *parsed.Chatter
	} else {
		classified := len(allDecisions) + len(allActionItems) + len(allUnresolved)
		result.ChatterCount = len(history) - classified
	}
	if result.ChatterCount < 0 {
		result.ChatterCount = 0
	}
	attachSummarySources(&result, history)
	return result, nil
}

func parseSummaryWire(value string) (summaryWire, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	value = strings.TrimSpace(value)
	var raw summaryWire
	if value == "" || json.Unmarshal([]byte(value), &raw) != nil {
		return summaryWire{}, false
	}
	raw.Decisions = normalizeSummaryItems(raw.Decisions)
	raw.ActionItems = normalizeSummaryItems(raw.ActionItems)
	raw.Unresolved = normalizeSummaryItems(raw.Unresolved)
	return raw, true
}

func normalizeSummaryItems(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`[]`)
	}
	var structured []summaryItem
	if json.Unmarshal(raw, &structured) == nil {
		encoded, _ := json.Marshal(structured)
		return encoded
	}
	var texts []string
	if json.Unmarshal(raw, &texts) != nil {
		return json.RawMessage(`[]`)
	}
	structured = make([]summaryItem, 0, len(texts))
	for _, text := range texts {
		if strings.TrimSpace(text) != "" {
			structured = append(structured, summaryItem{Text: strings.TrimSpace(text)})
		}
	}
	encoded, _ := json.Marshal(structured)
	return encoded
}

func limitSummaryItems(items json.RawMessage) []summaryItem {
	return limitSummaryItemList(decodeSummaryItems(items))
}

func decodeSummaryItems(items json.RawMessage) []summaryItem {
	var result []summaryItem
	if json.Unmarshal(items, &result) != nil {
		return []summaryItem{}
	}
	return result
}

func limitSummaryItemList(items []summaryItem) []summaryItem {
	filtered := make([]summaryItem, 0, minInt(len(items), maxSummaryItems))
	for _, item := range items {
		item.Text = strings.TrimSpace(item.Text)
		if item.Text != "" {
			filtered = append(filtered, item)
		}
		if len(filtered) == maxSummaryItems {
			break
		}
	}
	return filtered
}

func heuristicSummary(history []ai.Message) summaryWire {
	result := summaryWire{Decisions: json.RawMessage(`[]`), ActionItems: json.RawMessage(`[]`), Unresolved: json.RawMessage(`[]`)}
	decisions := make([]summaryItem, 0)
	actions := make([]summaryItem, 0)
	unresolved := make([]summaryItem, 0)
	for _, message := range history {
		id, text := summarySource(message.Body)
		lower := strings.ToLower(text)
		item := summaryItem{Text: text, SourceMessageID: id}
		switch {
		case strings.Contains(text, "決定") || strings.Contains(text, "確定") || strings.Contains(text, "採用") || strings.Contains(text, "リリース"):
			decisions = append(decisions, item)
		case strings.Contains(text, "確認") || strings.Contains(text, "レビュー") || strings.Contains(text, "対応") || strings.Contains(text, "お願いします") || strings.Contains(text, "してください"):
			actions = append(actions, item)
		case strings.Contains(text, "未解決") || strings.Contains(text, "検討") || strings.Contains(text, "課題") || strings.Contains(text, "どうする") || strings.Contains(text, "迷") || strings.Contains(lower, "?") || strings.Contains(text, "？"):
			unresolved = append(unresolved, item)
		}
	}
	encodedDecisions, _ := json.Marshal(decisions)
	encodedActions, _ := json.Marshal(actions)
	encodedUnresolved, _ := json.Marshal(unresolved)
	result.Summary = fmt.Sprintf("%d件の会話から、決まったこと・依頼・残っている論点をまとめました。", len(history))
	result.Decisions = encodedDecisions
	result.ActionItems = encodedActions
	result.Unresolved = encodedUnresolved
	chatter := len(history) - len(decisions) - len(actions) - len(unresolved)
	if chatter < 0 {
		chatter = 0
	}
	result.Chatter = &chatter
	return result
}

func summarySource(value string) (string, string) {
	const prefix = "[message_id="
	if !strings.HasPrefix(value, prefix) {
		return "", strings.TrimSpace(value)
	}
	end := strings.Index(value, "]")
	if end < len(prefix) {
		return "", strings.TrimSpace(value)
	}
	messageID := strings.Fields(value[len(prefix):end])
	if len(messageID) == 0 {
		return "", strings.TrimSpace(value[end+1:])
	}
	return messageID[0], strings.TrimSpace(value[end+1:])
}

func attachSummarySources(result *channelSummary, history []ai.Message) {
	allowed := make(map[string]struct{}, len(history))
	for _, message := range history {
		id, _ := summarySource(message.Body)
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	sanitize := func(items []summaryItem) []summaryItem {
		for index := range items {
			if _, exists := allowed[items[index].SourceMessageID]; !exists {
				items[index].SourceMessageID = ""
			}
		}
		return items
	}
	result.Decisions = sanitize(result.Decisions)
	result.ActionItems = sanitize(result.ActionItems)
	result.Unresolved = sanitize(result.Unresolved)
	used := make(map[string]struct{})
	for _, item := range append(append(append([]summaryItem{}, result.Decisions...), result.ActionItems...), result.Unresolved...) {
		if item.SourceMessageID != "" {
			used[item.SourceMessageID] = struct{}{}
		}
	}
	assign := func(items []summaryItem) []summaryItem {
		for index := range items {
			if items[index].SourceMessageID != "" {
				continue
			}
			for _, message := range history {
				id, text := summarySource(message.Body)
				if _, exists := used[id]; exists || id == "" {
					continue
				}
				if strings.Contains(text, items[index].Text) || items[index].Text == text {
					items[index].SourceMessageID = id
					used[id] = struct{}{}
					break
				}
			}
		}
		return items
	}
	result.Decisions = assign(result.Decisions)
	result.ActionItems = assign(result.ActionItems)
	result.Unresolved = assign(result.Unresolved)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
