# Database Design: Xu, Nhóm, Nhắn tin

## 1. Xu (Virtual Currency / Coins)

### 1.1 user_wallets
Ví xu của người dùng.

```sql
CREATE TABLE user_wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    balance BIGINT NOT NULL DEFAULT 0,           -- Số xu hiện tại
    total_earned BIGINT NOT NULL DEFAULT 0,      -- Tổng xu đã kiếm được
    total_spent BIGINT NOT NULL DEFAULT 0,       -- Tổng xu đã tiêu
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    
    CONSTRAINT user_wallets_user_id_unique UNIQUE(user_id),
    CONSTRAINT user_wallets_balance_non_negative CHECK (balance >= 0)
);

CREATE INDEX idx_user_wallets_user_id ON user_wallets(user_id);
```

### 1.2 coin_transactions
Lịch sử giao dịch xu.

```sql
CREATE TYPE coin_transaction_type AS ENUM (
    'EARN_LESSON_COMPLETE',    -- Hoàn thành bài học
    'EARN_QUIZ_PASS',          -- Vượt qua quiz
    'EARN_STREAK_BONUS',       -- Bonus streak
    'EARN_ACHIEVEMENT',        -- Đạt achievement
    'EARN_DAILY_LOGIN',        -- Đăng nhập hàng ngày
    'EARN_REFERRAL',           -- Giới thiệu bạn bè
    'EARN_PURCHASE',           -- Mua xu bằng tiền thật
    'SPEND_COURSE_UNLOCK',     -- Mở khóa khóa học
    'SPEND_HINT',              -- Mua gợi ý
    'SPEND_STREAK_FREEZE',     -- Mua freeze streak
    'SPEND_GIFT',              -- Tặng xu cho người khác
    'RECEIVE_GIFT',            -- Nhận xu từ người khác
    'REFUND',                  -- Hoàn xu
    'ADMIN_ADJUST'             -- Admin điều chỉnh
);

CREATE TABLE coin_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id UUID NOT NULL REFERENCES user_wallets(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type coin_transaction_type NOT NULL,
    amount BIGINT NOT NULL,                      -- Số xu (+/-)
    balance_after BIGINT NOT NULL,               -- Số dư sau giao dịch
    reference_type VARCHAR(50),                  -- 'lesson', 'course', 'user', etc.
    reference_id UUID,                           -- ID của entity liên quan
    description TEXT,
    metadata JSONB DEFAULT '{}',                 -- Dữ liệu bổ sung
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT coin_transactions_balance_after_non_negative CHECK (balance_after >= 0)
);

CREATE INDEX idx_coin_transactions_wallet_id ON coin_transactions(wallet_id);
CREATE INDEX idx_coin_transactions_user_id ON coin_transactions(user_id);
CREATE INDEX idx_coin_transactions_type ON coin_transactions(type);
CREATE INDEX idx_coin_transactions_created_at ON coin_transactions(created_at DESC);
CREATE INDEX idx_coin_transactions_reference ON coin_transactions(reference_type, reference_id);
```

### 1.3 coin_packages
Gói xu có thể mua bằng tiền thật.

```sql
CREATE TABLE coin_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    coin_amount BIGINT NOT NULL,                 -- Số xu nhận được
    bonus_amount BIGINT DEFAULT 0,               -- Xu bonus thêm
    price DECIMAL(12,2) NOT NULL,                -- Giá (VND)
    currency VARCHAR(3) DEFAULT 'VND',
    discount_percent INT DEFAULT 0,              -- % giảm giá
    is_featured BOOLEAN DEFAULT FALSE,           -- Gói nổi bật
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
```

### 1.4 coin_purchases
Lịch sử mua xu.

```sql
CREATE TYPE purchase_status AS ENUM ('PENDING', 'COMPLETED', 'FAILED', 'REFUNDED');

CREATE TABLE coin_purchases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    package_id UUID REFERENCES coin_packages(id),
    coin_amount BIGINT NOT NULL,                 -- Số xu mua
    bonus_amount BIGINT DEFAULT 0,
    price DECIMAL(12,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'VND',
    status purchase_status DEFAULT 'PENDING',
    payment_method VARCHAR(50),                  -- 'MBBANK', 'MOMO', etc.
    payment_reference VARCHAR(255),              -- Mã giao dịch thanh toán
    transaction_id UUID REFERENCES coin_transactions(id),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_coin_purchases_user_id ON coin_purchases(user_id);
CREATE INDEX idx_coin_purchases_status ON coin_purchases(status);
```

---

## 2. Nhóm (Groups)

### 2.1 groups
Thông tin nhóm.

```sql
CREATE TYPE group_type AS ENUM (
    'STUDY_GROUP',      -- Nhóm học tập
    'CLASS_GROUP',      -- Nhóm lớp học
    'COURSE_GROUP',     -- Nhóm khóa học
    'CUSTOM'            -- Nhóm tự tạo
);

CREATE TYPE group_privacy AS ENUM (
    'PUBLIC',           -- Ai cũng có thể tìm và tham gia
    'PRIVATE',          -- Cần được mời hoặc duyệt
    'SECRET'            -- Không hiển thị trong tìm kiếm
);

CREATE TABLE groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
    name VARCHAR(200) NOT NULL,
    slug VARCHAR(200) NOT NULL,
    description TEXT,
    avatar_url TEXT,
    cover_url TEXT,
    type group_type DEFAULT 'CUSTOM',
    privacy group_privacy DEFAULT 'PRIVATE',
    max_members INT DEFAULT 100,
    member_count INT DEFAULT 0,                  -- Denormalized for performance
    reference_type VARCHAR(50),                  -- 'class', 'course', etc.
    reference_id UUID,                           -- ID của class/course nếu có
    settings JSONB DEFAULT '{}',                 -- Cài đặt nhóm
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    
    CONSTRAINT groups_slug_unique UNIQUE(slug)
);

CREATE INDEX idx_groups_organization_id ON groups(organization_id);
CREATE INDEX idx_groups_type ON groups(type);
CREATE INDEX idx_groups_privacy ON groups(privacy);
CREATE INDEX idx_groups_reference ON groups(reference_type, reference_id);
CREATE INDEX idx_groups_created_by ON groups(created_by);
```

### 2.2 group_members
Thành viên nhóm.

```sql
CREATE TYPE group_member_role AS ENUM (
    'OWNER',            -- Chủ nhóm
    'ADMIN',            -- Quản trị viên
    'MODERATOR',        -- Điều hành viên
    'MEMBER'            -- Thành viên
);

CREATE TYPE group_member_status AS ENUM (
    'ACTIVE',           -- Đang hoạt động
    'PENDING',          -- Chờ duyệt
    'INVITED',          -- Được mời
    'BANNED',           -- Bị cấm
    'LEFT'              -- Đã rời nhóm
);

CREATE TABLE group_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role group_member_role DEFAULT 'MEMBER',
    status group_member_status DEFAULT 'ACTIVE',
    nickname VARCHAR(100),                       -- Biệt danh trong nhóm
    invited_by UUID REFERENCES users(id),
    joined_at TIMESTAMPTZ,
    last_read_at TIMESTAMPTZ,                    -- Đọc tin nhắn cuối lúc nào
    notification_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT group_members_unique UNIQUE(group_id, user_id)
);

CREATE INDEX idx_group_members_group_id ON group_members(group_id);
CREATE INDEX idx_group_members_user_id ON group_members(user_id);
CREATE INDEX idx_group_members_status ON group_members(status);
CREATE INDEX idx_group_members_role ON group_members(role);
```

### 2.3 group_join_requests
Yêu cầu tham gia nhóm (cho nhóm PRIVATE).

```sql
CREATE TYPE join_request_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED');

CREATE TABLE group_join_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message TEXT,                                -- Lời nhắn xin vào nhóm
    status join_request_status DEFAULT 'PENDING',
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT group_join_requests_unique UNIQUE(group_id, user_id)
);

CREATE INDEX idx_group_join_requests_group_id ON group_join_requests(group_id);
CREATE INDEX idx_group_join_requests_status ON group_join_requests(status);
```

---

## 3. Nhắn tin (Messaging)

### 3.1 conversations
Cuộc trò chuyện (hỗ trợ cả 1-1 và nhóm).

```sql
CREATE TYPE conversation_type AS ENUM (
    'DIRECT',           -- Chat 1-1
    'GROUP'             -- Chat nhóm (liên kết với groups table)
);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type conversation_type NOT NULL,
    group_id UUID REFERENCES groups(id) ON DELETE CASCADE,  -- NULL nếu DIRECT
    name VARCHAR(200),                           -- Tên conversation (cho DIRECT có thể NULL)
    last_message_id UUID,                        -- ID tin nhắn cuối (denormalized)
    last_message_at TIMESTAMPTZ,                 -- Thời gian tin nhắn cuối
    message_count BIGINT DEFAULT 0,              -- Tổng số tin nhắn
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    
    CONSTRAINT conversations_group_id_unique UNIQUE(group_id)
);

CREATE INDEX idx_conversations_type ON conversations(type);
CREATE INDEX idx_conversations_group_id ON conversations(group_id);
CREATE INDEX idx_conversations_last_message_at ON conversations(last_message_at DESC);
```

### 3.2 conversation_participants
Người tham gia cuộc trò chuyện.

```sql
CREATE TABLE conversation_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_message_id UUID,                   -- Tin nhắn đọc cuối
    last_read_at TIMESTAMPTZ,
    unread_count INT DEFAULT 0,                  -- Số tin chưa đọc
    is_muted BOOLEAN DEFAULT FALSE,
    muted_until TIMESTAMPTZ,
    is_pinned BOOLEAN DEFAULT FALSE,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    left_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT conversation_participants_unique UNIQUE(conversation_id, user_id)
);

CREATE INDEX idx_conversation_participants_conversation_id ON conversation_participants(conversation_id);
CREATE INDEX idx_conversation_participants_user_id ON conversation_participants(user_id);
CREATE INDEX idx_conversation_participants_unread ON conversation_participants(user_id, unread_count) WHERE unread_count > 0;
```

### 3.3 messages
Tin nhắn.

```sql
CREATE TYPE message_type AS ENUM (
    'TEXT',             -- Tin nhắn văn bản
    'IMAGE',            -- Hình ảnh
    'FILE',             -- File đính kèm
    'VIDEO',            -- Video
    'AUDIO',            -- Ghi âm
    'STICKER',          -- Sticker
    'SYSTEM',           -- Tin nhắn hệ thống
    'COIN_GIFT',        -- Tặng xu
    'SHARED_LESSON',    -- Chia sẻ bài học
    'SHARED_COURSE',    -- Chia sẻ khóa học
    'POLL'              -- Bình chọn
);

CREATE TYPE message_status AS ENUM (
    'SENDING',          -- Đang gửi
    'SENT',             -- Đã gửi
    'DELIVERED',        -- Đã nhận
    'FAILED'            -- Gửi thất bại
);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,  -- NULL cho system messages
    type message_type DEFAULT 'TEXT',
    content TEXT,                                -- Nội dung tin nhắn
    metadata JSONB DEFAULT '{}',                 -- Dữ liệu bổ sung (file info, shared content, etc.)
    reply_to_id UUID REFERENCES messages(id) ON DELETE SET NULL,  -- Tin nhắn được reply
    status message_status DEFAULT 'SENT',
    is_edited BOOLEAN DEFAULT FALSE,
    edited_at TIMESTAMPTZ,
    is_pinned BOOLEAN DEFAULT FALSE,
    pinned_by UUID REFERENCES users(id),
    pinned_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_messages_conversation_id ON messages(conversation_id);
CREATE INDEX idx_messages_sender_id ON messages(sender_id);
CREATE INDEX idx_messages_created_at ON messages(conversation_id, created_at DESC);
CREATE INDEX idx_messages_reply_to ON messages(reply_to_id) WHERE reply_to_id IS NOT NULL;
CREATE INDEX idx_messages_pinned ON messages(conversation_id, is_pinned) WHERE is_pinned = TRUE;
-- Full-text search cho tìm kiếm tin nhắn
CREATE INDEX idx_messages_content_search ON messages USING gin(to_tsvector('vietnamese', content));
```

### 3.4 message_reactions
Reactions cho tin nhắn.

```sql
CREATE TABLE message_reactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji VARCHAR(20) NOT NULL,                  -- Emoji unicode hoặc custom code
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT message_reactions_unique UNIQUE(message_id, user_id, emoji)
);

CREATE INDEX idx_message_reactions_message_id ON message_reactions(message_id);
CREATE INDEX idx_message_reactions_user_id ON message_reactions(user_id);
```

### 3.5 message_attachments
File đính kèm tin nhắn.

```sql
CREATE TABLE message_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    file_url TEXT NOT NULL,                      -- URL trên MinIO
    file_size BIGINT,                            -- Bytes
    mime_type VARCHAR(100),
    thumbnail_url TEXT,                          -- Thumbnail cho image/video
    metadata JSONB DEFAULT '{}',                 -- width, height, duration, etc.
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_message_attachments_message_id ON message_attachments(message_id);
```

### 3.6 message_mentions
Mentions trong tin nhắn (@user).

```sql
CREATE TABLE message_mentions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_offset INT,                            -- Vị trí bắt đầu trong content
    end_offset INT,                              -- Vị trí kết thúc
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT message_mentions_unique UNIQUE(message_id, user_id)
);

CREATE INDEX idx_message_mentions_message_id ON message_mentions(message_id);
CREATE INDEX idx_message_mentions_user_id ON message_mentions(user_id);
```

---

## 4. Entity Relationship Diagram

```
┌─────────────────┐         ┌─────────────────────┐
│     users       │────────▶│    user_wallets     │
└────────┬────────┘         └──────────┬──────────┘
         │                             │
         │                             ▼
         │                  ┌─────────────────────┐
         │                  │  coin_transactions  │
         │                  └─────────────────────┘
         │
         │         ┌─────────────────────┐
         ├────────▶│   coin_purchases    │
         │         └─────────────────────┘
         │
         │         ┌─────────────────┐         ┌─────────────────────┐
         ├────────▶│     groups      │◀───────▶│   conversations     │
         │         └────────┬────────┘         └──────────┬──────────┘
         │                  │                             │
         │                  ▼                             │
         │         ┌─────────────────────┐                │
         ├────────▶│   group_members     │                │
         │         └─────────────────────┘                │
         │                                                ▼
         │                                     ┌─────────────────────────────┐
         ├────────────────────────────────────▶│ conversation_participants  │
         │                                     └─────────────────────────────┘
         │
         │         ┌─────────────────┐
         └────────▶│    messages     │
                   └────────┬────────┘
                            │
         ┌──────────────────┼──────────────────┐
         ▼                  ▼                  ▼
┌─────────────────┐ ┌───────────────┐ ┌─────────────────────┐
│message_reactions│ │message_mentions│ │message_attachments │
└─────────────────┘ └───────────────┘ └─────────────────────┘
```

---

## 5. GORM Models (Go)

```go
// internal/model/wallet.go
type UserWallet struct {
    BaseModel
    UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
    Balance     int64     `gorm:"not null;default:0" json:"balance"`
    TotalEarned int64     `gorm:"not null;default:0" json:"total_earned"`
    TotalSpent  int64     `gorm:"not null;default:0" json:"total_spent"`
    
    User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

type CoinTransactionType string
const (
    CoinTxEarnLessonComplete CoinTransactionType = "EARN_LESSON_COMPLETE"
    CoinTxEarnQuizPass       CoinTransactionType = "EARN_QUIZ_PASS"
    CoinTxSpendCourseUnlock  CoinTransactionType = "SPEND_COURSE_UNLOCK"
    CoinTxSpendGift          CoinTransactionType = "SPEND_GIFT"
    CoinTxReceiveGift        CoinTransactionType = "RECEIVE_GIFT"
    // ... more types
)

type CoinTransaction struct {
    BaseModel
    WalletID      uuid.UUID           `gorm:"type:uuid;not null;index" json:"wallet_id"`
    UserID        uuid.UUID           `gorm:"type:uuid;not null;index" json:"user_id"`
    Type          CoinTransactionType `gorm:"type:coin_transaction_type;not null" json:"type"`
    Amount        int64               `gorm:"not null" json:"amount"`
    BalanceAfter  int64               `gorm:"not null" json:"balance_after"`
    ReferenceType *string             `gorm:"size:50" json:"reference_type,omitempty"`
    ReferenceID   *uuid.UUID          `gorm:"type:uuid" json:"reference_id,omitempty"`
    Description   *string             `json:"description,omitempty"`
    Metadata      datatypes.JSON      `gorm:"default:'{}'" json:"metadata"`
}

// internal/model/group.go
type GroupType string
const (
    GroupTypeStudy  GroupType = "STUDY_GROUP"
    GroupTypeClass  GroupType = "CLASS_GROUP"
    GroupTypeCourse GroupType = "COURSE_GROUP"
    GroupTypeCustom GroupType = "CUSTOM"
)

type GroupPrivacy string
const (
    GroupPrivacyPublic  GroupPrivacy = "PUBLIC"
    GroupPrivacyPrivate GroupPrivacy = "PRIVATE"
    GroupPrivacySecret  GroupPrivacy = "SECRET"
)

type Group struct {
    BaseModel
    OrganizationID *uuid.UUID   `gorm:"type:uuid;index" json:"organization_id,omitempty"`
    Name           string       `gorm:"size:200;not null" json:"name"`
    Slug           string       `gorm:"size:200;not null;uniqueIndex" json:"slug"`
    Description    *string      `json:"description,omitempty"`
    AvatarURL      *string      `json:"avatar_url,omitempty"`
    CoverURL       *string      `json:"cover_url,omitempty"`
    Type           GroupType    `gorm:"type:group_type;default:'CUSTOM'" json:"type"`
    Privacy        GroupPrivacy `gorm:"type:group_privacy;default:'PRIVATE'" json:"privacy"`
    MaxMembers     int          `gorm:"default:100" json:"max_members"`
    MemberCount    int          `gorm:"default:0" json:"member_count"`
    ReferenceType  *string      `gorm:"size:50" json:"reference_type,omitempty"`
    ReferenceID    *uuid.UUID   `gorm:"type:uuid" json:"reference_id,omitempty"`
    Settings       datatypes.JSON `gorm:"default:'{}'" json:"settings"`
    CreatedBy      uuid.UUID    `gorm:"type:uuid;not null" json:"created_by"`
    
    Creator *User          `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
    Members []GroupMember  `gorm:"foreignKey:GroupID" json:"members,omitempty"`
}

type GroupMemberRole string
const (
    GroupRoleOwner     GroupMemberRole = "OWNER"
    GroupRoleAdmin     GroupMemberRole = "ADMIN"
    GroupRoleModerator GroupMemberRole = "MODERATOR"
    GroupRoleMember    GroupMemberRole = "MEMBER"
)

type GroupMember struct {
    BaseModel
    GroupID             uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_group_user" json:"group_id"`
    UserID              uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_group_user" json:"user_id"`
    Role                GroupMemberRole  `gorm:"type:group_member_role;default:'MEMBER'" json:"role"`
    Status              string           `gorm:"type:group_member_status;default:'ACTIVE'" json:"status"`
    Nickname            *string          `gorm:"size:100" json:"nickname,omitempty"`
    JoinedAt            *time.Time       `json:"joined_at,omitempty"`
    LastReadAt          *time.Time       `json:"last_read_at,omitempty"`
    NotificationEnabled bool             `gorm:"default:true" json:"notification_enabled"`
    
    Group *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
    User  *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// internal/model/message.go
type ConversationType string
const (
    ConversationTypeDirect ConversationType = "DIRECT"
    ConversationTypeGroup  ConversationType = "GROUP"
)

type Conversation struct {
    BaseModel
    Type          ConversationType `gorm:"type:conversation_type;not null" json:"type"`
    GroupID       *uuid.UUID       `gorm:"type:uuid;uniqueIndex" json:"group_id,omitempty"`
    Name          *string          `gorm:"size:200" json:"name,omitempty"`
    LastMessageID *uuid.UUID       `gorm:"type:uuid" json:"last_message_id,omitempty"`
    LastMessageAt *time.Time       `json:"last_message_at,omitempty"`
    MessageCount  int64            `gorm:"default:0" json:"message_count"`
    
    Group        *Group                    `gorm:"foreignKey:GroupID" json:"group,omitempty"`
    Participants []ConversationParticipant `gorm:"foreignKey:ConversationID" json:"participants,omitempty"`
    LastMessage  *Message                  `gorm:"foreignKey:ID;references:LastMessageID" json:"last_message,omitempty"`
}

type ConversationParticipant struct {
    BaseModel
    ConversationID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_conv_user" json:"conversation_id"`
    UserID            uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_conv_user" json:"user_id"`
    LastReadMessageID *uuid.UUID `gorm:"type:uuid" json:"last_read_message_id,omitempty"`
    LastReadAt        *time.Time `json:"last_read_at,omitempty"`
    UnreadCount       int        `gorm:"default:0" json:"unread_count"`
    IsMuted           bool       `gorm:"default:false" json:"is_muted"`
    MutedUntil        *time.Time `json:"muted_until,omitempty"`
    IsPinned          bool       `gorm:"default:false" json:"is_pinned"`
    JoinedAt          time.Time  `gorm:"default:now()" json:"joined_at"`
    LeftAt            *time.Time `json:"left_at,omitempty"`
    
    Conversation *Conversation `gorm:"foreignKey:ConversationID" json:"conversation,omitempty"`
    User         *User         `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

type MessageType string
const (
    MessageTypeText        MessageType = "TEXT"
    MessageTypeImage       MessageType = "IMAGE"
    MessageTypeFile        MessageType = "FILE"
    MessageTypeVideo       MessageType = "VIDEO"
    MessageTypeAudio       MessageType = "AUDIO"
    MessageTypeSystem      MessageType = "SYSTEM"
    MessageTypeCoinGift    MessageType = "COIN_GIFT"
    MessageTypeSharedLesson MessageType = "SHARED_LESSON"
)

type Message struct {
    BaseModel
    ConversationID uuid.UUID      `gorm:"type:uuid;not null;index" json:"conversation_id"`
    SenderID       *uuid.UUID     `gorm:"type:uuid;index" json:"sender_id,omitempty"`
    Type           MessageType    `gorm:"type:message_type;default:'TEXT'" json:"type"`
    Content        *string        `json:"content,omitempty"`
    Metadata       datatypes.JSON `gorm:"default:'{}'" json:"metadata"`
    ReplyToID      *uuid.UUID     `gorm:"type:uuid;index" json:"reply_to_id,omitempty"`
    Status         string         `gorm:"type:message_status;default:'SENT'" json:"status"`
    IsEdited       bool           `gorm:"default:false" json:"is_edited"`
    EditedAt       *time.Time     `json:"edited_at,omitempty"`
    IsPinned       bool           `gorm:"default:false" json:"is_pinned"`
    PinnedBy       *uuid.UUID     `gorm:"type:uuid" json:"pinned_by,omitempty"`
    PinnedAt       *time.Time     `json:"pinned_at,omitempty"`
    
    Conversation *Conversation        `gorm:"foreignKey:ConversationID" json:"conversation,omitempty"`
    Sender       *User                `gorm:"foreignKey:SenderID" json:"sender,omitempty"`
    ReplyTo      *Message             `gorm:"foreignKey:ReplyToID" json:"reply_to,omitempty"`
    Attachments  []MessageAttachment  `gorm:"foreignKey:MessageID" json:"attachments,omitempty"`
    Reactions    []MessageReaction    `gorm:"foreignKey:MessageID" json:"reactions,omitempty"`
    Mentions     []MessageMention     `gorm:"foreignKey:MessageID" json:"mentions,omitempty"`
}
```

---

## 6. Lưu ý Implementation

### 6.1 Realtime với WebSocket
- Sử dụng `internal/socket/` hub hiện có
- Thêm channels: `chat:{conversation_id}`, `group:{group_id}`
- Events: `message.new`, `message.edit`, `message.delete`, `message.reaction`, `typing`

### 6.2 Tích hợp với LiveKit (Meet)
- Chat trong classroom đã có sẵn qua LiveKit data channel
- Persist messages vào DB cho history
- Sync giữa WebSocket hub và LiveKit data channel

### 6.3 Xu/Coins
- Sử dụng database transactions để đảm bảo tính toàn vẹn
- Lock row khi cập nhật balance để tránh race condition
- Emit events cho gamification system khi earn/spend

### 6.4 MinIO Buckets
Thêm bucket cho attachments:
- `study-chat-attachments` - File đính kèm chat
