package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound               = errors.New("resource not found")
	ErrConflict               = errors.New("resource already exists")
	ErrForbidden              = errors.New("forbidden")
	ErrChannelManageForbidden = errors.New("channel management forbidden")
	ErrNotMember              = errors.New("not a channel member")
	ErrUnauthorized           = errors.New("invalid credentials")
)

const (
	defaultChannelID       = "general"
	maxMessageBodyLength   = 10_000
	maxChannelNameLength   = 100
	maxChannelGroupLength  = 64
	maxChannelKindLength   = 32
	maxChannelDescription  = 500
	maxChannelMembers      = 100
	maxUserNameLength      = 80
	maxReactionLength      = 32
	maxJSONBodyBytes       = 128 << 10
	maxWebSocketFrameBytes = 64 << 10
)

var defaultPublicChannelIDs = []string{"general", "frontend", "design-system", "roadmap", "research"}

const deletedMessageBody = "このメッセージは削除されました。"

func isDefaultPublicChannel(channelID string) bool {
	for _, publicChannelID := range defaultPublicChannelIDs {
		if publicChannelID == channelID {
			return true
		}
	}
	return false
}

type validationError struct{ message string }

func (e validationError) Error() string { return e.message }

func invalidInput(format string, args ...any) error {
	return validationError{message: fmt.Sprintf(format, args...)}
}

func validateMessageBody(body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return invalidInput("body is required")
	}
	if utf8.RuneCountInString(body) > maxMessageBodyLength {
		return invalidInput("body must be %d characters or fewer", maxMessageBodyLength)
	}
	return nil
}

func validateChannelRequest(request channelRequest) error {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return invalidInput("name is required")
	}
	if utf8.RuneCountInString(name) > maxChannelNameLength {
		return invalidInput("name must be %d characters or fewer", maxChannelNameLength)
	}
	if utf8.RuneCountInString(strings.TrimSpace(request.Group)) > maxChannelGroupLength {
		return invalidInput("group must be %d characters or fewer", maxChannelGroupLength)
	}
	if utf8.RuneCountInString(strings.TrimSpace(request.Kind)) > maxChannelKindLength {
		return invalidInput("kind must be %d characters or fewer", maxChannelKindLength)
	}
	group := strings.TrimSpace(request.Group)
	if group != "" && group != "Engineering" && group != "Product" && group != "Direct messages" {
		return invalidInput("group is not supported")
	}
	kind := strings.TrimSpace(request.Kind)
	if kind != "" && kind != "channel" && kind != "dm" {
		return invalidInput("kind is not supported")
	}
	if utf8.RuneCountInString(strings.TrimSpace(request.Description)) > maxChannelDescription {
		return invalidInput("description must be %d characters or fewer", maxChannelDescription)
	}
	if err := validateChannelMemberIDs(request.MemberIDs); err != nil {
		return err
	}
	return nil
}

func validateChannelMemberIDs(memberIDs []string) error {
	if len(memberIDs) > maxChannelMembers {
		return invalidInput("member_ids must contain %d users or fewer", maxChannelMembers)
	}
	seenMembers := make(map[string]struct{}, len(memberIDs))
	for _, memberID := range memberIDs {
		memberID = strings.TrimSpace(memberID)
		if memberID == "" {
			return invalidInput("member_ids contains an empty user id")
		}
		if _, exists := seenMembers[memberID]; exists {
			return invalidInput("member_ids must not contain duplicates")
		}
		seenMembers[memberID] = struct{}{}
	}
	return nil
}

type channelUpdateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	MemberIDs   []string `json:"member_ids,omitempty"`
}

func validateChannelUpdateRequest(request channelUpdateRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return invalidInput("name is required")
	}
	if utf8.RuneCountInString(strings.TrimSpace(request.Name)) > maxChannelNameLength {
		return invalidInput("name must be %d characters or fewer", maxChannelNameLength)
	}
	if utf8.RuneCountInString(strings.TrimSpace(request.Description)) > maxChannelDescription {
		return invalidInput("description must be %d characters or fewer", maxChannelDescription)
	}
	if request.MemberIDs != nil {
		if err := validateChannelMemberIDs(request.MemberIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateReactionEmoji(emoji string) error {
	emoji = strings.TrimSpace(emoji)
	if emoji == "" {
		return invalidInput("emoji is required")
	}
	if utf8.RuneCountInString(emoji) > maxReactionLength {
		return invalidInput("emoji must be %d characters or fewer", maxReactionLength)
	}
	return nil
}

const orbitAIUserID = "u-orbit-ai"

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Handle   string `json:"handle"`
	Initials string `json:"initials"`
	Color    string `json:"color"`
	IsBot    bool   `json:"is_bot,omitempty"`
}

type PublicUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Handle   string `json:"handle"`
	Initials string `json:"initials"`
	Color    string `json:"color"`
}

type ChannelMember struct {
	PublicUser
	Role  string `json:"role"`
	IsBot bool   `json:"is_bot,omitempty"`
}

type userRecord struct {
	User
	PasswordHash string
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Unread      int    `json:"unread"`
	Presence    string `json:"presence,omitempty"`
	Initials    string `json:"initials,omitempty"`
	Color       string `json:"color,omitempty"`
}

type Reaction struct {
	Emoji   string `json:"emoji"`
	Count   int    `json:"count"`
	Reacted bool   `json:"reacted,omitempty"`
}

type Message struct {
	ID              string     `json:"id"`
	ChannelID       string     `json:"channel_id"`
	AuthorID        string     `json:"author_id"`
	Author          string     `json:"author"`
	Initials        string     `json:"initials"`
	Color           string     `json:"color"`
	Time            string     `json:"time"`
	Body            string     `json:"body"`
	Edited          bool       `json:"edited,omitempty"`
	Deleted         bool       `json:"deleted,omitempty"`
	Reactions       []Reaction `json:"reactions,omitempty"`
	ThreadCount     int        `json:"thread_count,omitempty"`
	ParentMessageID string     `json:"parent_message_id,omitempty"`
	Sequence        int64      `json:"-"`
}

type messageRequest struct {
	Body            string `json:"body"`
	ParentMessageID string `json:"parent_message_id,omitempty"`
}

type reactionRequest struct {
	Emoji string `json:"emoji"`
}

type channelRequest struct {
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	MemberIDs   []string `json:"member_ids,omitempty"`
}

type updateMessageRequest struct {
	Body string `json:"body"`
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateProfileRequest struct {
	Name string `json:"name"`
}

type realtimeEvent struct {
	Type            string   `json:"type"`
	ChannelID       string   `json:"channel_id"`
	EventID         int64    `json:"event_id"`
	Sequence        int64    `json:"sequence"`
	Message         *Message `json:"message,omitempty"`
	MessageID       string   `json:"message_id,omitempty"`
	ParentMessageID string   `json:"parent_message_id,omitempty"`
	Delta           string   `json:"delta,omitempty"`
	Error           string   `json:"error,omitempty"`
	ActorID         string   `json:"actor_id,omitempty"`
	ActorName       string   `json:"actor_name,omitempty"`
	ActorHandle     string   `json:"actor_handle,omitempty"`
	ActorInitials   string   `json:"actor_initials,omitempty"`
	ActorColor      string   `json:"actor_color,omitempty"`
	Presence        string   `json:"presence,omitempty"`
	MemberID        string   `json:"member_id,omitempty"`
}

type EventRecord struct {
	Sequence int64
	Event    realtimeEvent
}

type MessagePage struct {
	Messages   []Message `json:"messages"`
	NextCursor string    `json:"next_cursor,omitempty"`
	HasMore    bool      `json:"has_more"`
	Cursor     int64     `json:"cursor"`
}

type EventPage struct {
	Events     []realtimeEvent `json:"events"`
	NextCursor string          `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
	Cursor     int64           `json:"cursor"`
}

func cursorValue(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func cursorString(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

type repository interface {
	Close()
	ListUsers(context.Context) ([]PublicUser, error)
	ListChannelMembers(context.Context, string) ([]ChannelMember, error)
	ListChannels(context.Context, string) ([]Channel, int64, error)
	HasChannel(context.Context, string) (bool, error)
	IsChannelMember(context.Context, string, string) (bool, error)
	ListChannelMemberIDs(context.Context, string) (map[string]struct{}, error)
	ChannelIDForMessage(context.Context, string) (string, error)
	CreateChannel(context.Context, string, channelRequest) (Channel, []EventRecord, error)
	UpdateChannel(context.Context, string, string, channelUpdateRequest) (Channel, []EventRecord, error)
	MarkChannelRead(context.Context, string, string) (int64, error)
	ListMessagePage(context.Context, string, string, int) (MessagePage, error)
	ListAIContextMessages(context.Context, string, int) ([]Message, error)
	ListUnreadMessages(context.Context, string, string) ([]Message, error)
	ListUnreadMessageContext(context.Context, string, string, int) ([]Message, int, error)
	ConsumeAIDailyQuota(context.Context, string, time.Time, int) (bool, error)
	ListThreadPage(context.Context, string, string, int) (MessagePage, error)
	ListEvents(context.Context, string, int64, int) (EventPage, error)
	CreateMessage(context.Context, string, string, messageRequest) (Message, EventRecord, error)
	UpdateMessage(context.Context, string, string, string) (Message, EventRecord, error)
	DeleteMessage(context.Context, string, string) (string, EventRecord, error)
	AddReaction(context.Context, string, string, string) (Message, EventRecord, error)
	RemoveReaction(context.Context, string, string, string) (Message, EventRecord, error)
	RegisterUser(context.Context, registerRequest) (User, error)
	AuthenticateUser(context.Context, string, string) (User, error)
	UpdateUserProfile(context.Context, string, updateProfileRequest) (User, error)
	FindUserBySession(context.Context, string) (User, error)
	CreateSession(context.Context, string) (string, error)
	DeleteSession(context.Context, string) error
}
