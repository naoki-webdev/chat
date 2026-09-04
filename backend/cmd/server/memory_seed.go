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
		sequence:         seededSequence(),
		channels:         seededChannels(),
		messages:         seededMessages(),
		messageSequences: seededMessageSequences(),
		owners:           seededMessageOwners(),
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

func (r *memoryRepository) Close() {}
