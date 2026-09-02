package main

import "golang.org/x/crypto/bcrypt"

func newMemoryRepository() *memoryRepository {
	users := map[string]userRecord{
		"u-naoki":     {User: User{ID: "u-naoki", Name: "Taro Tanaka", Email: "demo@example.com", Handle: "taro", Initials: "TT", Color: "linear-gradient(135deg, #f3a683, #c56cf0)"}, PasswordHash: mustHashPassword("demo-password")},
		"u-ayaka":     {User: User{ID: "u-ayaka", Name: "Ayaka Mori", Email: "ayaka@example.com", Handle: "ayaka", Initials: "AM", Color: "linear-gradient(135deg, #f8c291, #e55039)"}, PasswordHash: mustHashPassword("demo-password")},
		"u-ken":       {User: User{ID: "u-ken", Name: "Ken Ito", Email: "ken@example.com", Handle: "ken", Initials: "KI", Color: "linear-gradient(135deg, #82ccdd, #60a3bc)"}, PasswordHash: mustHashPassword("demo-password")},
		"u-mio":       {User: User{ID: "u-mio", Name: "Mio Tanaka", Email: "mio@example.com", Handle: "mio", Initials: "MT", Color: "linear-gradient(135deg, #b8e994, #78e08f)"}, PasswordHash: mustHashPassword("demo-password")},
		orbitAIUserID: {User: User{ID: orbitAIUserID, Name: "Orbit AI", Email: "orbit-ai@local", Handle: "orbit-ai", Initials: "✦", Color: "linear-gradient(135deg, #8b5cf6, #22d3ee)", IsBot: true}, PasswordHash: mustHashPassword("demo-password")},
	}
	return &memoryRepository{
		sequence:         7,
		channels:         seededChannels(),
		messages:         seededMessages(),
		messageSequences: map[string]int64{"g-1": 1, "g-2": 2, "f-1": 3, "ds-1": 4, "ds-r1": 5, "ds-r2": 6, "ds-r3": 7},
		owners:           map[string]string{"g-1": "u-ken", "g-2": "u-naoki", "f-1": "u-ayaka", "ds-1": "u-ayaka", "ds-r1": "u-ken", "ds-r2": "u-naoki", "ds-r3": "u-ayaka"},
		events:           []EventRecord{},
		readStates:       make(map[string]map[string]int64),
		readMessageIDs:   make(map[string]map[string]string),
		reactionUsers:    make(map[string]map[string]map[string]struct{}),
		memberships:      seededMemberships(),
		users:            users,
		byEmail:          map[string]string{"demo@example.com": "u-naoki", "ayaka@example.com": "u-ayaka", "ken@example.com": "u-ken", "mio@example.com": "u-mio", "orbit-ai@local": orbitAIUserID},
		sessions:         make(map[string]memorySession),
		aiDailyUsage:     make(map[string]int),
	}
}

func seededMemberships() map[string]map[string]string {
	memberships := make(map[string]map[string]string)
	add := func(channelID, userID, role string) {
		if memberships[channelID] == nil {
			memberships[channelID] = make(map[string]string)
		}
		memberships[channelID][userID] = role
	}

	for _, channel := range seededChannels() {
		if channel.Kind != "channel" {
			continue
		}
		for _, userID := range []string{"u-naoki", "u-ayaka", "u-ken", "u-mio", orbitAIUserID} {
			role := "member"
			if userID == "u-naoki" {
				role = "owner"
			}
			add(channel.ID, userID, role)
		}
	}
	for _, userID := range []string{"u-naoki", "u-ayaka", orbitAIUserID} {
		add("ayaka", userID, "member")
	}
	for _, userID := range []string{"u-naoki", "u-ken", orbitAIUserID} {
		add("ken", userID, "member")
	}
	add("orbit-ai", "u-naoki", "member")
	add("orbit-ai", orbitAIUserID, "member")
	return memberships
}

func mustHashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
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

func seededMessages() map[string][]Message {
	return map[string][]Message{
		"general":       {{ID: "g-1", ChannelID: "general", AuthorID: "u-ken", Author: "Ken Ito", Initials: "KI", Color: "#82ccdd", Time: "08:30", Body: "おはようございます。今週もよろしくお願いします！", Reactions: []Reaction{{Emoji: "☀️", Count: 5}}}, {ID: "g-2", ChannelID: "general", AuthorID: "u-naoki", Author: "Taro Tanaka", Initials: "TT", Color: "#c56cf0", Time: "08:33", Body: "おはよう！リアルタイムチャットの初期画面を作り始めます。", Reactions: []Reaction{{Emoji: "🚀", Count: 2}}}},
		"frontend":      {{ID: "f-1", ChannelID: "frontend", AuthorID: "u-ayaka", Author: "Ayaka Mori", Initials: "AM", Color: "#f8c291", Time: "昨日", Body: "APIレスポンスの型定義、shared/typesに置いておくと使いやすそうです。", Reactions: []Reaction{{Emoji: "👍", Count: 3}}}},
		"design-system": {{ID: "ds-1", ChannelID: "design-system", AuthorID: "u-ayaka", Author: "Ayaka Mori", Initials: "AM", Color: "#f8c291", Time: "09:42", Body: "新しいカラートークンをまとめました。", Reactions: []Reaction{{Emoji: "✨", Count: 4}}, ThreadCount: 3}, {ID: "ds-r1", ChannelID: "design-system", AuthorID: "u-ken", Author: "Ken Ito", Initials: "KI", Color: "#82ccdd", Time: "09:50", Body: "カードの境界線は、もう少し薄くしてもよさそうです。", ParentMessageID: "ds-1"}, {ID: "ds-r2", ChannelID: "design-system", AuthorID: "u-naoki", Author: "Taro Tanaka", Initials: "TT", Color: "#c56cf0", Time: "09:55", Body: "了解です。余白とのバランスを見て調整します。", ParentMessageID: "ds-1"}, {ID: "ds-r3", ChannelID: "design-system", AuthorID: "u-ayaka", Author: "Ayaka Mori", Initials: "AM", Color: "#f8c291", Time: "10:05", Body: "明日のレビューで最終確認しましょう。", ParentMessageID: "ds-1"}},
		"roadmap":       {}, "research": {}, "ayaka": {}, "ken": {}, "orbit-ai": {},
	}
}

func (r *memoryRepository) Close() {}
