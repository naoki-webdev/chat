package main

import (
	"encoding/json"
	"time"
)

type demoSeedMessage struct {
	ID              string
	ChannelID       string
	AuthorID        string
	Author          string
	Initials        string
	Color           string
	Time            string
	Body            string
	Reactions       []Reaction
	ThreadCount     int
	ParentMessageID string
	CreatedAgo      time.Duration
}

func seededChannels() []Channel {
	return []Channel{
		{ID: "general", Name: "general", Group: "Engineering", Kind: "channel", Description: "プロジェクトの最新情報と雑談"},
		{ID: "frontend", Name: "frontend", Group: "Engineering", Kind: "channel", Description: "フロントエンド開発の相談場所"},
		{ID: "design-system", Name: "design-system", Group: "Engineering", Kind: "channel", Description: "OrbitのUIとデザイントークン"},
		{ID: "roadmap", Name: "roadmap", Group: "Product", Kind: "channel", Description: "プロダクトの方向性を話す場所"},
		{ID: "research", Name: "user-research", Group: "Product", Kind: "channel", Description: "ユーザーインタビューと発見"},
		{ID: "ayaka", Name: "Ayaka Mori", Group: "Direct messages", Kind: "dm", PeerUserID: "u-ayaka", Presence: "online", Initials: "AM", Color: "#f8c291"},
		{ID: "ken", Name: "Ken Ito", Group: "Direct messages", Kind: "dm", PeerUserID: "u-ken", Presence: "away", Initials: "KI", Color: "#82ccdd"},
		{ID: "orbit-ai", Name: "Orbit AI", Group: "Direct messages", Kind: "dm", PeerUserID: orbitAIUserID, Presence: "online", Initials: "✦", Color: "linear-gradient(135deg, #8b5cf6, #22d3ee)", Description: "リアルタイム会話に参加するAIアシスタント"},
	}
}

func demoSeedMessages() []demoSeedMessage {
	return []demoSeedMessage{
		{ID: "g-1", ChannelID: "general", AuthorID: "u-ken", Author: "Ken Ito", Initials: "KI", Color: "#82ccdd", Time: "08:30", Body: "おはようございます。今週もよろしくお願いします！", Reactions: []Reaction{{Emoji: "☀️", Count: 5}}, CreatedAgo: -5 * time.Hour},
		{ID: "g-2", ChannelID: "general", AuthorID: "u-naoki", Author: "Taro Tanaka", Initials: "TT", Color: "#c56cf0", Time: "08:33", Body: "おはよう！リアルタイムチャットの初期画面を作り始めます。", Reactions: []Reaction{{Emoji: "🚀", Count: 2}}, CreatedAgo: -4*time.Hour - 57*time.Minute},
		{ID: "f-1", ChannelID: "frontend", AuthorID: "u-ayaka", Author: "Ayaka Mori", Initials: "AM", Color: "#f8c291", Time: "昨日", Body: "APIレスポンスの型定義、shared/typesに置いておくと使いやすそうです。", Reactions: []Reaction{{Emoji: "👍", Count: 3}}, CreatedAgo: -24 * time.Hour},
		{ID: "ds-1", ChannelID: "design-system", AuthorID: "u-ayaka", Author: "Ayaka Mori", Initials: "AM", Color: "#f8c291", Time: "09:42", Body: "新しいカラートークンをまとめました。", Reactions: []Reaction{{Emoji: "✨", Count: 4}}, ThreadCount: 3, CreatedAgo: -3 * time.Hour},
		{ID: "ds-r1", ChannelID: "design-system", AuthorID: "u-ken", Author: "Ken Ito", Initials: "KI", Color: "#82ccdd", Time: "09:50", Body: "カードの境界線は、もう少し薄くしてもよさそうです。", ParentMessageID: "ds-1", CreatedAgo: -2*time.Hour - 52*time.Minute},
		{ID: "ds-r2", ChannelID: "design-system", AuthorID: "u-naoki", Author: "Taro Tanaka", Initials: "TT", Color: "#c56cf0", Time: "09:55", Body: "了解です。余白とのバランスを見て調整します。", ParentMessageID: "ds-1", CreatedAgo: -2*time.Hour - 47*time.Minute},
		{ID: "ds-r3", ChannelID: "design-system", AuthorID: "u-ayaka", Author: "Ayaka Mori", Initials: "AM", Color: "#f8c291", Time: "10:05", Body: "明日のレビューで最終確認しましょう。", ParentMessageID: "ds-1", CreatedAgo: -2*time.Hour - 37*time.Minute},
	}
}

func seededMessageSequences() map[string]int64 {
	sequences := make(map[string]int64)
	for index, message := range demoSeedMessages() {
		sequences[message.ID] = int64(index + 1)
	}
	return sequences
}

func seededMessageOwners() map[string]string {
	owners := make(map[string]string)
	for _, message := range demoSeedMessages() {
		owners[message.ID] = message.AuthorID
	}
	return owners
}

func seededMessages() map[string][]Message {
	messages := map[string][]Message{}
	for _, seed := range demoSeedMessages() {
		messages[seed.ChannelID] = append(messages[seed.ChannelID], Message{
			ID: seed.ID, ChannelID: seed.ChannelID, AuthorID: seed.AuthorID, Author: seed.Author,
			Initials: seed.Initials, Color: seed.Color, Time: seed.Time, Body: seed.Body,
			Reactions: seed.Reactions, ThreadCount: seed.ThreadCount, ParentMessageID: seed.ParentMessageID,
		})
	}
	for _, channel := range seededChannels() {
		if _, ok := messages[channel.ID]; !ok {
			messages[channel.ID] = []Message{}
		}
	}
	return messages
}

func seededSequence() int64 {
	return int64(len(demoSeedMessages()))
}

func seedReactionsJSON(reactions []Reaction) string {
	payload, _ := json.Marshal(reactions)
	return string(payload)
}
