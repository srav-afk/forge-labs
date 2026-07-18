package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type APIKey struct {
	ID            string `gorm:"primaryKey;type:text"`
	KeyHash       string `gorm:"uniqueIndex;not null"`
	ClientID      string `gorm:"index;not null"`
	MaxConcurrent int
	Enabled       bool
	CreatedAt     time.Time
}

func (APIKey) TableName() string { return "gateway_api_keys" }

type ClientIdentity struct {
	ClientID      string
	MaxConcurrent int
	KeyID         string
}

type KeyStore struct {
	db *gorm.DB

	mu      sync.RWMutex
	byHash  map[string]ClientIdentity
	envOnly bool
}

func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func NewKeyStore(db *gorm.DB) *KeyStore {
	return &KeyStore{db: db, byHash: map[string]ClientIdentity{}}
}

func (s *KeyStore) LoadFromEnv(csv string, single string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byHash == nil {
		s.byHash = map[string]ClientIdentity{}
	}
	if single != "" {
		s.byHash[HashAPIKey(single)] = ClientIdentity{
			ClientID: "default", MaxConcurrent: 32, KeyID: "env-default",
		}
	}
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, client := part, "client"
		maxC := 32
		if i := strings.IndexByte(part, ':'); i >= 0 {
			key = part[:i]
			rest := part[i+1:]
			if j := strings.IndexByte(rest, ':'); j >= 0 {
				client = rest[:j]
				_, _ = fmtSscanf(rest[j+1:], &maxC)
			} else {
				client = rest
			}
		}
		if key == "" {
			continue
		}
		if maxC <= 0 {
			maxC = 32
		}
		s.byHash[HashAPIKey(key)] = ClientIdentity{
			ClientID: client, MaxConcurrent: maxC, KeyID: "env-" + client,
		}
	}
	if len(s.byHash) > 0 && s.db == nil {
		s.envOnly = true
	}
}

func fmtSscanf(s string, n *int) (int, error) {
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + int(c-'0')
	}
	if v > 0 {
		*n = v
	}
	return 1, nil
}

func (s *KeyStore) Reload(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	var rows []APIKey
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return err
	}
	next := map[string]ClientIdentity{}
	s.mu.RLock()
	for h, id := range s.byHash {
		if strings.HasPrefix(id.KeyID, "env-") {
			next[h] = id
		}
	}
	s.mu.RUnlock()
	for _, r := range rows {
		maxC := r.MaxConcurrent
		if maxC <= 0 {
			maxC = 32
		}
		next[r.KeyHash] = ClientIdentity{
			ClientID: r.ClientID, MaxConcurrent: maxC, KeyID: r.ID,
		}
	}
	s.mu.Lock()
	s.byHash = next
	s.mu.Unlock()
	return nil
}

func (s *KeyStore) Required() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byHash) > 0
}

func (s *KeyStore) Lookup(rawKey string) (ClientIdentity, bool) {
	if rawKey == "" {
		return ClientIdentity{}, false
	}
	h := HashAPIKey(rawKey)
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byHash[h]
	return id, ok
}

func (s *KeyStore) Upsert(ctx context.Context, id, rawKey, clientID string, maxConcurrent int) error {
	if s.db == nil {
		return gorm.ErrInvalidData
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 32
	}
	row := APIKey{
		ID:            id,
		KeyHash:       HashAPIKey(rawKey),
		ClientID:      clientID,
		MaxConcurrent: maxConcurrent,
		Enabled:       true,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return err
	}
	return s.Reload(ctx)
}
