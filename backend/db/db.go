package db

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	GORM *gorm.DB
}

func Open(path string) (*DB, error) {
	gormDB, err := gorm.Open(sqlite.Open(path+"?_journal_mode=WAL&_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)

	db := &DB{GORM: gormDB}
	if err := db.autoMigrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	sqlDB, err := db.GORM.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (db *DB) autoMigrate() error {
	if err := db.GORM.AutoMigrate(&Session{}, &ProjectRef{}, &Message{}, &Agent{}, &ProjectAgent{}, &Knowledge{}); err != nil {
		return err
	}

	// Schema version via PRAGMA user_version (default 0 = uninitialized)
	var schemaVersion int
	db.GORM.Raw("PRAGMA user_version").Scan(&schemaVersion)

	if schemaVersion < 1 {
		// Migrate FTS5 from old (content, tags) to (title) — check if title column exists
		needMigrate := true
		rows, err := db.GORM.Raw("PRAGMA table_info('knowledge_fts')").Rows()
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cid int; var name, colType string; var notNull int; var dflt *string; var pk int
				rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk)
				if name == "title" {
					needMigrate = false
					break
				}
			}
		}
		if needMigrate {
			db.GORM.Exec("DROP TABLE IF EXISTS knowledge_fts")
		}
		if err := db.GORM.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(title, tokenize='unicode61')").Error; err != nil {
			return err
		}
		// Re-index existing entries
		var knowledges []Knowledge
		db.GORM.Select("id, title").Find(&knowledges)
		for _, k := range knowledges {
			if k.Title != "" {
				db.GORM.Exec("INSERT OR IGNORE INTO knowledge_fts(rowid, title) VALUES(?, ?)", k.ID, k.Title)
			}
		}
		// Mark migration done
		db.GORM.Exec("PRAGMA user_version = 1")
	}

	return nil
}

// ── Session ──

type Session struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	ModelID         string    `gorm:"size:64;default:''" json:"model_id"`
	Status          string    `gorm:"size:16;default:idle" json:"status"`
	Rounds          int       `gorm:"default:0" json:"rounds"`
	ContextUsagePct int       `gorm:"column:context_usage_pct;default:0" json:"context_usage_pct"`
	ContextStartID  int64     `gorm:"column:context_start_id;default:0" json:"context_start_id"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Session) TableName() string { return "sessions" }

// ── ProjectRef ──

type ProjectRef struct {
	ID        string         `gorm:"primaryKey;size:64" json:"id"`
	Title     string         `gorm:"default:''" json:"title"`
	WorkDir   string         `gorm:"column:work_dir" json:"work_dir"`
	Status    string         `gorm:"size:16;default:idle" json:"status"`
	Agents    []ProjectAgent `gorm:"foreignKey:ProjectID" json:"agents"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
}

func (ProjectRef) TableName() string { return "project_refs" }

// ── ProjectAgent ──

type ProjectAgent struct {
	ID             int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID      string `gorm:"size:64;index" json:"project_id"`
	Name           string `gorm:"size:128" json:"name"`
	SessionID      string `gorm:"size:64" json:"session_id"`
	Role           string `gorm:"size:32" json:"role"`
	Status         string `gorm:"size:16;default:idle" json:"status"`
	Prompt         string `gorm:"default:''" json:"prompt"`
	EnableBrowser  bool   `gorm:"column:enable_browser;default:0" json:"enable_browser"`
	EnableDesktop  bool   `gorm:"column:enable_desktop;default:0" json:"enable_desktop"`
}

func (ProjectAgent) TableName() string { return "project_agents" }

// ── Message ──

type Message struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID        string    `gorm:"size:64;index:idx_messages_session,priority:1" json:"session_id"`
	Role             string    `gorm:"size:16" json:"role"`
	Content          string    `gorm:"default:''" json:"content"`
	MultiContent     string    `gorm:"column:multi_content;default:''" json:"multi_content"`
	ToolCalls        string    `gorm:"column:tool_calls;default:'[]'" json:"tool_calls"`
	ToolCallID       string    `gorm:"size:64;default:'';column:tool_call_id" json:"tool_call_id"`
	ReasoningContent string    `gorm:"column:reasoning_content;default:''" json:"reasoning_content"`
	RoleLabel        string    `gorm:"size:64;default:'';column:role_label" json:"role_label"`
	Hidden           bool      `gorm:"default:0" json:"hidden"`
	Bookmarked       bool      `gorm:"default:0" json:"bookmarked"`
	CreatedAt        time.Time `gorm:"autoCreateTime;index:idx_messages_session,priority:2" json:"created_at"`
}

func (Message) TableName() string { return "messages" }

// ── Agent ──

type Agent struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:128;uniqueIndex" json:"name"`
	Desc      string    `gorm:"size:512;default:''" json:"desc"`
	Content   string    `gorm:"default:''" json:"content"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Agent) TableName() string { return "agents" }

// ── Knowledge ──

type Knowledge struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Title     string    `gorm:"default:''" json:"title"`
	Content   string    `gorm:"default:''" json:"content"`
	Source    string    `gorm:"size:128;default:''" json:"source"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Knowledge) TableName() string { return "knowledges" }

// ── Session methods ──

func (db *DB) CreateSession(id, status string) error {
	return db.GORM.Create(&Session{
		ID:     id,
		Status: status,
	}).Error
}

func (db *DB) UpdateSessionStatus(id, status string) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).Update("status", status).Error
}

func (db *DB) ResetAllSessionStatuses() error {
	return db.GORM.Model(&Session{}).Where("status = ? AND id != ?", "running", "main").Update("status", "idle").Error
}

func (db *DB) UpdateSessionRounds(id string, rounds, contextPct int) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).Updates(map[string]any{
		"rounds":            rounds,
		"context_usage_pct": contextPct,
	}).Error
}

func (db *DB) UpdateSessionModel(id, modelID string) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).Update("model_id", modelID).Error
}

func (db *DB) UpdateSessionContextStart(id string, startID int64) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).Update("context_start_id", startID).Error
}

func (db *DB) GetLastMessageID(sessionID string) (int64, error) {
	var id int64
	err := db.GORM.Model(&Message{}).Select("COALESCE(MAX(id), 0)").Where("session_id = ?", sessionID).Scan(&id).Error
	return id, err
}

func (db *DB) GetSession(id string) (*Session, error) {
	var s Session
	if err := db.GORM.First(&s, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) CountRunningSessions() (int, error) {
	var count int64
	err := db.GORM.Model(&Session{}).Where("status = 'running' AND id != 'main'").Count(&count).Error
	return int(count), err
}

func (db *DB) ResumeSession(id string) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).Update("status", "running").Error
}

// ── Message methods ──

func (db *DB) InsertMessage(m *Message) (int64, error) {
	if err := db.GORM.Create(m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (db *DB) GetMessages(sessionID string, afterID int64) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("session_id = ? AND id > ?", sessionID, afterID).
		Order("id ASC").
		Find(&msgs).Error
	return msgs, err
}

func (db *DB) GetRecentMessages(sessionID string, limit int) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("session_id = ?", sessionID).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, err
}

func (db *DB) GetMessagesBefore(sessionID string, beforeID int64, limit int) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("session_id = ? AND id < ?", sessionID, beforeID).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, err
}

func (db *DB) SearchMessages(query string, limit int) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("content LIKE ? AND role != 'system'", "%"+query+"%").
		Order("id DESC").Limit(limit).Find(&msgs).Error
	return msgs, err
}

func (db *DB) SetBookmark(msgID int64, bookmarked bool) error {
	return db.GORM.Model(&Message{}).Where("id = ?", msgID).Update("bookmarked", bookmarked).Error
}

func (db *DB) GetBookmarkedMessages(sessionIDs []string) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("session_id IN ? AND bookmarked = 1", sessionIDs).
		Order("id DESC").
		Find(&msgs).Error
	return msgs, err
}

func (db *DB) CleanupZombieSessions() error {
	if err := db.GORM.Model(&Session{}).Where("status = 'running' AND id != 'main'").
		Update("status", "idle").Error; err != nil {
		return err
	}
	if err := db.GORM.Model(&ProjectAgent{}).Where("status = 'running'").
		Update("status", "idle").Error; err != nil {
		return err
	}
	return db.GORM.Model(&ProjectRef{}).Where("status = 'running'").
		Update("status", "paused").Error
}

// ── Project methods ──

func (db *DB) CreateProject(id, title, workDir string) error {
	return db.GORM.Create(&ProjectRef{
		ID:      id,
		Title:   title,
		WorkDir: workDir,
		Status:  "running",
	}).Error
}

func (db *DB) UpdateProjectStatus(id, status string) error {
	return db.GORM.Model(&ProjectRef{}).Where("id = ?", id).Update("status", status).Error
}

func (db *DB) GetProject(id string) (*ProjectRef, error) {
	var p ProjectRef
	if err := db.GORM.Preload("Agents").First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) ListProjects() ([]ProjectRef, error) {
	var refs []ProjectRef
	err := db.GORM.Preload("Agents").Order("created_at DESC").Find(&refs).Error
	return refs, err
}

// ── ProjectAgent methods ──

func (db *DB) AddProjectAgent(projectID, name, sessionID, role, prompt string, enableBrowser, enableDesktop bool) error {
	return db.GORM.Create(&ProjectAgent{
		ProjectID:     projectID,
		Name:          name,
		SessionID:     sessionID,
		Role:          role,
		Status:        "running",
		Prompt:        prompt,
		EnableBrowser: enableBrowser,
		EnableDesktop: enableDesktop,
	}).Error
}

func (db *DB) UpdateProjectAgentStatus(name, sessionID, status string) error {
	return db.GORM.Model(&ProjectAgent{}).Where("name = ? AND session_id = ?", name, sessionID).Update("status", status).Error
}

func (db *DB) ResetAllAgentStatuses() error {
	return db.GORM.Model(&ProjectAgent{}).Where("status = ?", "running").Update("status", "idle").Error
}

func (db *DB) RemoveProjectAgent(sessionID string) error {
	return db.GORM.Where("session_id = ?", sessionID).Delete(&ProjectAgent{}).Error
}

func (db *DB) DeleteProjectSessions(projectID string) error {
	var agents []ProjectAgent
	if err := db.GORM.Where("project_id = ?", projectID).Find(&agents).Error; err != nil {
		return err
	}
	for _, a := range agents {
		db.DeleteMessages(a.SessionID)
		db.DeleteSession(a.SessionID)
	}
	return nil
}

func (db *DB) DeleteProject(id string) error {
	if err := db.GORM.Where("project_id = ?", id).Delete(&ProjectAgent{}).Error; err != nil {
		return fmt.Errorf("delete project agents: %w", err)
	}
	if err := db.GORM.Where("id = ?", id).Delete(&ProjectRef{}).Error; err != nil {
		return fmt.Errorf("delete project ref: %w", err)
	}
	return nil
}

func (db *DB) GetProjectAgent(sessionID string) (*ProjectAgent, error) {
	var a ProjectAgent
	if err := db.GORM.First(&a, "session_id = ?", sessionID).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (db *DB) GetProjectAgents(projectID string) ([]ProjectAgent, error) {
	var agents []ProjectAgent
	err := db.GORM.Where("project_id = ?", projectID).Order("id ASC").Find(&agents).Error
	return agents, err
}

func (db *DB) DeleteMessages(sessionID string) error {
	return db.GORM.Where("session_id = ?", sessionID).Delete(&Message{}).Error
}

func (db *DB) DeleteSession(id string) error {
	return db.GORM.Where("id = ?", id).Delete(&Session{}).Error
}

// ── Agent methods ──

func (db *DB) CreateAgent(name, content string) error {
	return db.GORM.Create(&Agent{
		Name:    name,
		Content: content,
	}).Error
}

func (db *DB) UpdateAgent(name, content string) error {
	return db.GORM.Model(&Agent{}).Where("name = ?", name).Update("content", content).Error
}

func (db *DB) UpdateAgentDesc(name, desc string) error {
	return db.GORM.Model(&Agent{}).Where("name = ?", name).Update("desc", desc).Error
}

func (db *DB) DeleteAgent(name string) error {
	return db.GORM.Where("name = ?", name).Delete(&Agent{}).Error
}

func (db *DB) ListAgents() ([]Agent, error) {
	var agents []Agent
	err := db.GORM.Order("name ASC").Find(&agents).Error
	return agents, err
}

func (db *DB) GetAgentByName(name string) (*Agent, error) {
	var a Agent
	if err := db.GORM.Where("name = ?", name).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ── Knowledge methods ──

func (db *DB) InsertKnowledge(title, content, source string) (int64, error) {
	k := &Knowledge{Title: title, Content: content, Source: source}
	if err := db.GORM.Create(k).Error; err != nil {
		return 0, err
	}
	// Sync to FTS5
	if err := db.GORM.Exec("INSERT INTO knowledge_fts(rowid, title) VALUES(?, ?)", k.ID, title).Error; err != nil {
		return 0, err
	}
	return k.ID, nil
}

func (db *DB) SearchKnowledge(query string, limit int) ([]Knowledge, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	// Count total FTS5 matches
	var count int64
	if err := db.GORM.Raw(
		"SELECT COUNT(*) FROM knowledge_fts WHERE knowledge_fts MATCH ?", query,
	).Scan(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	if count > 100 {
		return nil, fmt.Errorf("结果过多(%d条)，添加更多关键词", count)
	}

	// Get matching rowids ordered by BM25 rank
	var ids []int64
	if err := db.GORM.Raw(
		"SELECT rowid FROM knowledge_fts WHERE knowledge_fts MATCH ? ORDER BY rank LIMIT ?",
		query, limit,
	).Scan(&ids).Error; err != nil {
		return nil, err
	}

	var knowledge []Knowledge
	if err := db.GORM.Where("id IN ?", ids).Order("created_at DESC").Find(&knowledge).Error; err != nil {
		return nil, err
	}

	// If 11-100 results, return title-only (content stripped) to prompt LLM refinement
	if count > 10 {
		for i := range knowledge {
			knowledge[i].Content = ""
		}
	}

	return knowledge, nil
}

func (db *DB) ListKnowledge(limit, offset int) ([]Knowledge, error) {
	var knowledge []Knowledge
	err := db.GORM.Order("created_at DESC").Limit(limit).Offset(offset).Find(&knowledge).Error
	return knowledge, err
}

func (db *DB) UpdateKnowledge(id int64, title, content string) error {
	updates := map[string]any{}
	if title != "" {
		updates["title"] = title
	}
	if content != "" {
		updates["content"] = content
	}
	if len(updates) == 0 {
		return nil
	}
	if err := db.GORM.Model(&Knowledge{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	if title != "" {
		// Re-sync FTS5 title
		db.GORM.Exec("DELETE FROM knowledge_fts WHERE rowid = ?", id)
		if err := db.GORM.Exec("INSERT INTO knowledge_fts(rowid, title) VALUES(?, ?)", id, title).Error; err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) DeleteKnowledge(id int64) error {
	if err := db.GORM.Where("id = ?", id).Delete(&Knowledge{}).Error; err != nil {
		return err
	}
	return db.GORM.Exec("DELETE FROM knowledge_fts WHERE rowid = ?", id).Error
}

func (db *DB) KnowledgeCount() (int64, error) {
	var count int64
	err := db.GORM.Model(&Knowledge{}).Count(&count).Error
	return count, err
}

func (db *DB) SearchKnowledgeByQuery(q string, limit, offset int) ([]Knowledge, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	var knowledge []Knowledge
	var count int64
	query := "%" + q + "%"
	if err := db.GORM.Model(&Knowledge{}).Where("title LIKE ? OR content LIKE ?", query, query).Count(&count).Error; err != nil {
		return nil, 0, err
	}
	err := db.GORM.Where("title LIKE ? OR content LIKE ?", query, query).
		Order("created_at DESC").Limit(limit).Offset(offset).Find(&knowledge).Error
	return knowledge, count, err
}
