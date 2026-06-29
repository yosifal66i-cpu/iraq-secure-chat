package types

import (
	"time"

	"github.com/google/uuid"
)

// Domain types
type UserID uuid.UUID
type ChatID uuid.UUID
type MessageID uuid.UUID
type MediaID uuid.UUID
type CallID uuid.UUID
type SessionID uuid.UUID
type BotID uuid.UUID

type User struct {
	ID          UserID    `json:"id"`
	Phone       string    `json:"phone,omitempty"`
	Email       string    `json:"email,omitempty"`
	Username    string    `json:"username,omitempty"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Premium     bool      `json:"premium"`
	LastSeen    time.Time `json:"last_seen"`
	CreatedAt   time.Time `json:"created_at"`
	Settings    UserSettings `json:"settings"`
}

type UserSettings struct {
	LastSeenPrivacy      string `json:"last_seen_privacy"`      // everyone, contacts, nobody
	ProfilePhotoPrivacy  string `json:"profile_photo_privacy"`
	PhonePrivacy         string `json:"phone_privacy"`
	GroupAddPrivacy      string `json:"group_add_privacy"`
	ShowForwardedSender  bool   `json:"show_forwarded_sender"`
	NotificationsEnabled bool   `json:"notifications_enabled"`
	DNDEnabled           bool   `json:"dnd_enabled"`
	DNDStart             string `json:"dnd_start,omitempty"`
	DNDEnd               string `json:"dnd_end,omitempty"`
	Language             string `json:"language"`
}

type Chat struct {
	ID          ChatID    `json:"id"`
	Type        ChatType  `json:"type"`     // private, group, supergroup, channel, bot
	Title       string    `json:"title,omitempty"`
	Username    string    `json:"username,omitempty"`
	Description string    `json:"description,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	CreatedBy   UserID    `json:"created_by"`
	Settings    ChatSettings `json:"settings"`
	CreatedAt   time.Time `json:"created_at"`
}

type ChatType string

const (
	ChatTypePrivate    ChatType = "private"
	ChatTypeGroup      ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
	ChatTypeChannel    ChatType = "channel"
	ChatTypeBot        ChatType = "bot"
)

type ChatSettings struct {
	SlowModeSeconds      int  `json:"slow_mode_seconds,omitempty"`
	SignMessages         bool `json:"sign_messages"`
	JoinByLink           bool `json:"join_by_link"`
	HiddenMembers        bool `json:"hidden_members"`
	NoForwards           bool `json:"no_forwards"`
	TopicsEnabled        bool `json:"topics_enabled"`
	AutoDeleteMessageTTL int  `json:"auto_delete_message_ttl,omitempty"` // seconds
}

type ChatMember struct {
	ChatID      ChatID    `json:"chat_id"`
	UserID      UserID    `json:"user_id"`
	Role        MemberRole `json:"role"`
	Permissions Permissions `json:"permissions"`
	JoinedAt    time.Time `json:"joined_at"`
}

type MemberRole string

const (
	MemberRoleOwner     MemberRole = "owner"
	MemberRoleAdmin     MemberRole = "admin"
	MemberRoleMember    MemberRole = "member"
	MemberRoleRestricted MemberRole = "restricted"
	MemberRoleBanned    MemberRole = "banned"
)

type Permissions struct {
	CanSendMessages   bool `json:"can_send_messages"`
	CanSendMedia      bool `json:"can_send_media"`
	CanSendStickers   bool `json:"can_send_stickers"`
	CanSendPolls      bool `json:"can_send_polls"`
	CanAddMembers     bool `json:"can_add_members"`
	CanPinMessages    bool `json:"can_pin_messages"`
	CanChangeInfo     bool `json:"can_change_info"`
	CanDeleteMessages bool `json:"can_delete_messages"`
}

type Message struct {
	ID           MessageID         `json:"message_id"`
	ChatID       ChatID            `json:"chat_id"`
	SenderID     UserID            `json:"sender_id"`
	Type         MessageType       `json:"type"`
	Content      string            `json:"content"`
	ReplyTo      *MessageID        `json:"reply_to,omitempty"`
	ForwardFrom  *ForwardInfo      `json:"forward_from,omitempty"`
	Media        *MediaAttachment  `json:"media,omitempty"`
	Poll         *Poll             `json:"poll,omitempty"`
	EditedAt     *time.Time        `json:"edited_at,omitempty"`
	DeletedFor   []UserID          `json:"deleted_for,omitempty"`
	DeletedAll   bool              `json:"deleted_for_all"`
	Reactions    map[string][]UserID `json:"reactions,omitempty"`
	Entities     []MessageEntity   `json:"entities,omitempty"`
	SentAt       time.Time         `json:"sent_at"`
	ScheduleAt   *time.Time        `json:"schedule_at,omitempty"`
	AutoDeleteAt *time.Time        `json:"auto_delete_at,omitempty"`
	IDempotencyKey string          `json:"idempotency_key,omitempty"`
}

type MessageType string

const (
	MsgTypeText    MessageType = "text"
	MsgTypePhoto   MessageType = "photo"
	MsgTypeVideo   MessageType = "video"
	MsgTypeAudio   MessageType = "audio"
	MsgTypeFile    MessageType = "file"
	MsgTypeSticker MessageType = "sticker"
	MsgTypeGIF     MessageType = "gif"
	MsgTypeLocation MessageType = "location"
	MsgTypeContact MessageType = "contact"
	MsgTypePoll    MessageType = "poll"
	MsgTypeCall    MessageType = "call"
	MsgTypeService MessageType = "service"
)

type MessageEntity struct {
	Type   string `json:"type"`   // bold, italic, code, pre, link, mention, bot_command, hashtag
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

type ForwardInfo struct {
	FromChatID ChatID `json:"from_chat_id"`
	FromMsgID  MessageID `json:"from_message_id"`
	SenderID   UserID `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	HideSender bool   `json:"hide_sender"`
}

type MediaAttachment struct {
	ID           MediaID `json:"id"`
	Type         string  `json:"type"`          // photo, video, audio, file
	MimeType     string  `json:"mime_type"`
	URL          string  `json:"url"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
	FileName     string  `json:"file_name,omitempty"`
	FileSize     int64   `json:"file_size"`
	Width        int     `json:"width,omitempty"`
	Height       int     `json:"height,omitempty"`
	Duration     int     `json:"duration,omitempty"` // seconds
}

type Poll struct {
	ID        string       `json:"id"`
	Question  string       `json:"question"`
	Options   []PollOption `json:"options"`
	IsClosed  bool         `json:"is_closed"`
	IsAnonymous bool       `json:"is_anonymous"`
	MultipleChoice bool    `json:"multiple_choice"`
	TotalVotes int         `json:"total_votes"`
}

type PollOption struct {
	Text       string `json:"text"`
	VoterCount int    `json:"voter_count"`
	Voted      bool   `json:"voted,omitempty"`
}

type MessageDeliveryStatus string

const (
	StatusSending   MessageDeliveryStatus = "sending"
	StatusSent      MessageDeliveryStatus = "sent"
	StatusDelivered MessageDeliveryStatus = "delivered"
	StatusRead      MessageDeliveryStatus = "read"
	StatusFailed    MessageDeliveryStatus = "failed"
)

// WebSocket frames
type WSClientFrame struct {
	Type    string      `json:"type"`
	ChatIDs []ChatID    `json:"chat_ids,omitempty"`
	ChatID  ChatID      `json:"chat_id,omitempty"`
	MsgID   MessageID   `json:"message_id,omitempty"`
	Action  string      `json:"action,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type WSServerFrame struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// Presence
type PresenceStatus string

const (
	PresenceOnline  PresenceStatus = "online"
	PresenceOffline PresenceStatus = "offline"
)

type PresenceEvent struct {
	UserID   UserID         `json:"user_id"`
	Status   PresenceStatus `json:"status"`
	LastSeen int64          `json:"last_seen,omitempty"` // unix timestamp
}

// API Response
type APIResponse struct {
	Ok    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error *APIError   `json:"error,omitempty"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewAPIResponse(data interface{}) *APIResponse {
	return &APIResponse{Ok: true, Data: data}
}

func NewAPIError(code int, message string) *APIResponse {
	return &APIResponse{Ok: false, Error: &APIError{Code: code, Message: message}}
}

// Pagination
type CursorPagination struct {
	Cursor    string `json:"cursor,omitempty"`
	Direction string `json:"direction"` // before, after
	Limit     int    `json:"limit"`
}

type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}
