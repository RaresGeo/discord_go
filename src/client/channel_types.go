package client

type Overwrite struct {
	ID    Snowflake `json:"id"`
	Type  int       `json:"type"`
	Allow string    `json:"allow"`
	Deny  string    `json:"deny"`
}

type ThreadMetadata struct {
	Archived            bool    `json:"archived"`
	AutoArchiveDuration int     `json:"auto_archive_duration"`
	ArchiveTimestamp    string  `json:"archive_timestamp"`
	Locked              bool    `json:"locked"`
	Invitable           *bool   `json:"invitable,omitempty"`
	CreateTimestamp     *string `json:"create_timestamp,omitempty"`
}

type ThreadMember struct {
	ID            *Snowflake `json:"id,omitempty"`
	UserID        *Snowflake `json:"user_id,omitempty"`
	JoinTimestamp string     `json:"join_timestamp"`
	Flags         int        `json:"flags"`
}

type Tag struct {
	ID        Snowflake  `json:"id"`
	Name      string     `json:"name"`
	Moderated bool       `json:"moderated"`
	EmojiID   *Snowflake `json:"emoji_id,omitempty"`
	EmojiName *string    `json:"emoji_name,omitempty"`
}

type DefaultReaction struct {
	EmojiID   *Snowflake `json:"emoji_id,omitempty"`
	EmojiName *string    `json:"emoji_name,omitempty"`
}

type User struct {
	ID            Snowflake `json:"id"`
	Username      string    `json:"username"`
	Discriminator string    `json:"discriminator"`
	Avatar        *string   `json:"avatar"`
	Bot           *bool     `json:"bot,omitempty"`
	System        *bool     `json:"system,omitempty"`
	MFAEnabled    *bool     `json:"mfa_enabled,omitempty"`
	Banner        *string   `json:"banner,omitempty"`
	AccentColor   *int      `json:"accent_color,omitempty"`
	Locale        *string   `json:"locale,omitempty"`
	Verified      *bool     `json:"verified,omitempty"`
	Email         *string   `json:"email,omitempty"`
	Flags         *int      `json:"flags,omitempty"`
	PremiumType   *int      `json:"premium_type,omitempty"`
	PublicFlags   *int      `json:"public_flags,omitempty"`
}

type Channel struct {
	ID                            Snowflake        `json:"id"`
	Type                          int              `json:"type"`
	GuildID                       *Snowflake       `json:"guild_id,omitempty"`
	Position                      *int             `json:"position,omitempty"`
	PermissionOverwrites          []Overwrite      `json:"permission_overwrites,omitempty"`
	Name                          *string          `json:"name,omitempty"`
	Topic                         *string          `json:"topic,omitempty"`
	NSFW                          *bool            `json:"nsfw,omitempty"`
	LastMessageID                 *Snowflake       `json:"last_message_id,omitempty"`
	Bitrate                       *int             `json:"bitrate,omitempty"`
	UserLimit                     *int             `json:"user_limit,omitempty"`
	RateLimitPerUser              *int             `json:"rate_limit_per_user,omitempty"`
	Recipients                    []User           `json:"recipients,omitempty"`
	Icon                          *string          `json:"icon,omitempty"`
	OwnerID                       *Snowflake       `json:"owner_id,omitempty"`
	ApplicationID                 *Snowflake       `json:"application_id,omitempty"`
	Managed                       *bool            `json:"managed,omitempty"`
	ParentID                      *Snowflake       `json:"parent_id,omitempty"`
	LastPinTimestamp              *string          `json:"last_pin_timestamp,omitempty"` // ISO8601
	RTCRegion                     *string          `json:"rtc_region,omitempty"`
	VideoQualityMode              *int             `json:"video_quality_mode,omitempty"`
	MessageCount                  *int             `json:"message_count,omitempty"`
	MemberCount                   *int             `json:"member_count,omitempty"`
	ThreadMetadata                *ThreadMetadata  `json:"thread_metadata,omitempty"`
	Member                        *ThreadMember    `json:"member,omitempty"`
	DefaultAutoArchiveDuration    *int             `json:"default_auto_archive_duration,omitempty"`
	Permissions                   *string          `json:"permissions,omitempty"`
	Flags                         *int             `json:"flags,omitempty"`
	TotalMessageSent              *int             `json:"total_message_sent,omitempty"`
	AvailableTags                 []Tag            `json:"available_tags,omitempty"`
	AppliedTags                   []Snowflake      `json:"applied_tags,omitempty"`
	DefaultReactionEmoji          *DefaultReaction `json:"default_reaction_emoji,omitempty"`
	DefaultThreadRateLimitPerUser *int             `json:"default_thread_rate_limit_per_user,omitempty"`
	DefaultSortOrder              *int             `json:"default_sort_order,omitempty"`
	DefaultForumLayout            *int             `json:"default_forum_layout,omitempty"`
}
