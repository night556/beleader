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
	if err := db.GORM.AutoMigrate(&Thread{}, &Message{}, &Agent{}, &Knowledge{}, &MCPServer{}, &ModelProfile{}, &Runtime{}); err != nil {
		return err
	}

	var schemaVersion int
	db.GORM.Raw("PRAGMA user_version").Scan(&schemaVersion)

	if schemaVersion < 1 {
		db.GORM.Exec("DROP TABLE IF EXISTS knowledge_fts")
		if err := db.GORM.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(title, tokenize='unicode61')").Error; err != nil {
			return err
		}
		var knowledges []Knowledge
		db.GORM.Select("id, title").Find(&knowledges)
		for _, k := range knowledges {
			if k.Title != "" {
				db.GORM.Exec("INSERT OR IGNORE INTO knowledge_fts(rowid, title) VALUES(?, ?)", k.ID, k.Title)
			}
		}
		db.GORM.Exec("PRAGMA user_version = 1")
	}

	db.seedDefaultAgent()
	return nil
}

// ── Thread ──

type Thread struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	Title     string    `gorm:"default:''" json:"title"`
	AgentID   int64     `gorm:"default:0" json:"agent_id"`
	ModelID   string    `gorm:"size:64;default:''" json:"model_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Thread) TableName() string { return "threads" }

// ── Message ──

type Message struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID         string    `gorm:"size:64;index:idx_messages_thread,priority:1;column:thread_id" json:"thread_id"`
	Kind             string    `gorm:"size:32" json:"kind"`
	Content          string    `gorm:"default:''" json:"content"`
	MultiContent     string    `gorm:"column:multi_content;default:''" json:"multi_content"`
	ToolCalls        string    `gorm:"column:tool_calls;default:'[]'" json:"tool_calls"`
	ToolCallID       string    `gorm:"size:64;default:'';column:tool_call_id" json:"tool_call_id"`
	ReasoningContent string    `gorm:"column:reasoning_content;default:''" json:"reasoning_content"`
	CreatedAt        time.Time `gorm:"autoCreateTime;index:idx_messages_thread,priority:2" json:"created_at"`
}

func (Message) TableName() string { return "messages" }

// ── Agent ──

type Agent struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string    `gorm:"size:128;uniqueIndex" json:"name"`
	Desc           string    `gorm:"size:512;default:''" json:"desc"`
	SystemPrompt   string    `gorm:"column:system_prompt;default:''" json:"system_prompt"`
	Tools          string    `gorm:"type:text;default:'[]'" json:"tools"`
	DefaultModelID string    `gorm:"size:64;default:'';column:default_model_id" json:"default_model_id"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
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

// ── MCPServer ──

type MCPServer struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex" json:"name"`
	Type      string    `gorm:"size:16" json:"type"`
	Enabled   bool      `gorm:"default:false" json:"enabled"`
	Command   string    `gorm:"size:512;default:''" json:"command"`
	Args      string    `gorm:"type:text;default:'[]'" json:"args"`
	Env       string    `gorm:"type:text;default:'{}'" json:"env"`
	URL       string    `gorm:"size:512;default:''" json:"url"`
	Headers   string    `gorm:"type:text;default:'{}'" json:"headers"`
	Status    string    `gorm:"size:16;default:'disconnected'" json:"status"`
	Error     string    `gorm:"size:512;default:''" json:"error"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (MCPServer) TableName() string { return "mcp_servers" }

// ── ModelProfile ──

type ModelProfile struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"-"`
	ModelID      string `gorm:"size:64;uniqueIndex;column:model_id" json:"id"`
	BaseURL      string `gorm:"size:512;default:'';column:base_url" json:"base_url"`
	APIKey       string `gorm:"size:512;default:'';column:api_key" json:"api_key"`
	Model        string `gorm:"size:128;default:'';column:model" json:"model"`
	Vision       bool   `gorm:"default:false" json:"vision"`
	ContextLimit    int    `gorm:"default:128000;column:context_limit" json:"context_limit"`
	ReasoningEffort string `gorm:"size:16;default:'high';column:reasoning_effort" json:"reasoning_effort"`
	IsActive        bool   `gorm:"default:false;column:is_active" json:"-"`
}

func (ModelProfile) TableName() string { return "model_profiles" }

// ── Runtime ──

type Runtime struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"size:128;uniqueIndex" json:"name"`
	URL           string    `gorm:"size:256" json:"url"`
	Status        string    `gorm:"size:16;default:'active'" json:"status"`
	LastHeartbeat time.Time `gorm:"autoCreateTime" json:"last_heartbeat"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Runtime) TableName() string { return "runtimes" }

func (db *DB) ListModels() ([]ModelProfile, error) {
	var models []ModelProfile
	err := db.GORM.Order("id ASC").Find(&models).Error
	return models, err
}

func (db *DB) ActiveModel() (*ModelProfile, error) {
	var m ModelProfile
	if err := db.GORM.Where("is_active = 1").First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) GetModelByID(modelID string) (*ModelProfile, error) {
	var m ModelProfile
	if err := db.GORM.Where("model_id = ?", modelID).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) SetModels(models []ModelProfile) error {
	return db.GORM.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1=1").Delete(&ModelProfile{}).Error; err != nil {
			return err
		}
		for i := range models {
			models[i].ID = 0 // force insert
			models[i].IsActive = false
		}
		if len(models) > 0 {
			if err := tx.Create(&models).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) SetActiveModel(modelID string) error {
	return db.GORM.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ModelProfile{}).Where("1=1").Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Model(&ModelProfile{}).Where("model_id = ?", modelID).Update("is_active", true).Error
	})
}

// ── Seed ──

func (db *DB) seedDefaultAgent() {
	var count int64
	defaultTools := `["read_file","read_dir","write_file","edit_file","delete_file","search_content","search_files","read_status","update_status","run_command","web_search","web_fetch","run_http_request","spawn_worker"]`
	if db.GORM.Model(&Agent{}).Where("name = 'Default'").Count(&count); count == 0 {
		db.GORM.Create(&Agent{
			Name:         "Default",
			Desc:         "General-purpose assistant — read, write, edit files, run commands, search code and web",
			SystemPrompt: "You are a helpful AI assistant. You can read and write files, run shell commands, search the web, and spawn sub-agents for complex tasks.\n\n## How to work\n- Before editing files, read them first to understand the current state\n- After making changes, verify they work — run tests or check the build\n- When a task is complex, use spawn_worker to delegate sub-tasks\n- When done, summarize what you accomplished",
			Tools:        defaultTools,
		})
	}
}

// ── Thread methods ──

func (db *DB) CreateThread(id, title string, agentID int64, modelID string) error {
	return db.GORM.Create(&Thread{
		ID: id, Title: title, AgentID: agentID, ModelID: modelID,
	}).Error
}

func (db *DB) GetThread(id string) (*Thread, error) {
	var t Thread
	if err := db.GORM.First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (db *DB) ListThreads() ([]Thread, error) {
	var threads []Thread
	err := db.GORM.Order("updated_at DESC").Find(&threads).Error
	return threads, err
}

func (db *DB) DeleteThread(id string) error {
	db.DeleteMessages(id)
	return db.GORM.Where("id = ?", id).Delete(&Thread{}).Error
}

func (db *DB) UpdateThreadTitle(id, title string) error {
	return db.GORM.Model(&Thread{}).Where("id = ?", id).Update("title", title).Error
}

// ── Message methods ──

func (db *DB) InsertMessage(m *Message) (int64, error) {
	if err := db.GORM.Create(m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (db *DB) GetMessages(threadID string, afterID int64) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("thread_id = ? AND id > ?", threadID, afterID).
		Order("id ASC").
		Find(&msgs).Error
	return msgs, err
}

func (db *DB) GetRecentMessages(threadID string, limit int) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("thread_id = ?", threadID).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, err
}

func (db *DB) DeleteMessages(threadID string) error {
	return db.GORM.Where("thread_id = ?", threadID).Delete(&Message{}).Error
}

func (db *DB) SearchMessages(query string, limit int) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("content LIKE ? AND kind != 'notice'", "%"+query+"%").
		Order("id DESC").Limit(limit).Find(&msgs).Error
	return msgs, err
}

// ── Agent methods ──

func (db *DB) CreateAgent(name, desc, systemPrompt, tools, defaultModelID string) error {
	return db.GORM.Create(&Agent{
		Name: name, Desc: desc, SystemPrompt: systemPrompt, Tools: tools, DefaultModelID: defaultModelID,
	}).Error
}

func (db *DB) UpdateAgent(id int64, name, desc, systemPrompt, tools, defaultModelID string) error {
	return db.GORM.Model(&Agent{}).Where("id = ?", id).Updates(map[string]any{
		"name":             name,
		"desc":             desc,
		"system_prompt":    systemPrompt,
		"tools":            tools,
		"default_model_id": defaultModelID,
	}).Error
}

func (db *DB) DeleteAgent(id int64) error {
	return db.GORM.Where("id = ?", id).Delete(&Agent{}).Error
}

func (db *DB) ListAgents() ([]Agent, error) {
	var agents []Agent
	err := db.GORM.Order("name ASC").Find(&agents).Error
	return agents, err
}

func (db *DB) GetAgent(id int64) (*Agent, error) {
	var a Agent
	if err := db.GORM.First(&a, id).Error; err != nil {
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
	var count int64
	if err := db.GORM.Raw("SELECT COUNT(*) FROM knowledge_fts WHERE knowledge_fts MATCH ?", query).Scan(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	if count > 100 {
		return nil, fmt.Errorf("结果过多(%d条)，添加更多关键词", count)
	}
	var ids []int64
	if err := db.GORM.Raw("SELECT rowid FROM knowledge_fts WHERE knowledge_fts MATCH ? ORDER BY rank LIMIT ?", query, limit).Scan(&ids).Error; err != nil {
		return nil, err
	}
	var knowledge []Knowledge
	if err := db.GORM.Where("id IN ?", ids).Order("created_at DESC").Find(&knowledge).Error; err != nil {
		return nil, err
	}
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
		db.GORM.Exec("DELETE FROM knowledge_fts WHERE rowid = ?", id)
		db.GORM.Exec("INSERT INTO knowledge_fts(rowid, title) VALUES(?, ?)", id, title)
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

// ── MCPServer methods ──

func (db *DB) CreateMCPServer(s *MCPServer) error {
	return db.GORM.Create(s).Error
}

func (db *DB) UpdateMCPServer(s *MCPServer) error {
	return db.GORM.Model(&MCPServer{}).Where("id = ?", s.ID).Updates(map[string]any{
		"name":    s.Name,
		"type":    s.Type,
		"enabled": s.Enabled,
		"command": s.Command,
		"args":    s.Args,
		"env":     s.Env,
		"url":     s.URL,
		"headers": s.Headers,
		"status":  s.Status,
		"error":   s.Error,
	}).Error
}

func (db *DB) DeleteMCPServer(id int64) error {
	return db.GORM.Where("id = ?", id).Delete(&MCPServer{}).Error
}

func (db *DB) GetMCPServerByID(id int64) (*MCPServer, error) {
	var s MCPServer
	if err := db.GORM.First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) ListMCPServers() ([]MCPServer, error) {
	var servers []MCPServer
	err := db.GORM.Order("name ASC").Find(&servers).Error
	return servers, err
}

func (db *DB) ListEnabledMCPServers() ([]MCPServer, error) {
	var servers []MCPServer
	err := db.GORM.Where("enabled = 1").Order("name ASC").Find(&servers).Error
	return servers, err
}

// ── Runtime methods ──

func (db *DB) UpsertRuntime(name, url string) (*Runtime, error) {
	var r Runtime
	err := db.GORM.Where("name = ?", name).First(&r).Error
	if err != nil {
		r = Runtime{Name: name, URL: url, Status: "active", LastHeartbeat: time.Now()}
		if createErr := db.GORM.Create(&r).Error; createErr != nil {
			return nil, createErr
		}
		return &r, nil
	}
	r.URL = url
	r.Status = "active"
	r.LastHeartbeat = time.Now()
	if updateErr := db.GORM.Save(&r).Error; updateErr != nil {
		return nil, updateErr
	}
	return &r, nil
}

func (db *DB) UpdateRuntimeHeartbeat(id int64, status string) error {
	updates := map[string]any{"last_heartbeat": time.Now()}
	if status != "" {
		updates["status"] = status
	}
	return db.GORM.Model(&Runtime{}).Where("id = ?", id).Updates(updates).Error
}

func (db *DB) ListRuntimes() ([]Runtime, error) {
	var runtimes []Runtime
	err := db.GORM.Order("name ASC").Find(&runtimes).Error
	return runtimes, err
}

func (db *DB) GetRuntime(id int64) (*Runtime, error) {
	var r Runtime
	if err := db.GORM.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (db *DB) DeleteRuntime(id int64) error {
	return db.GORM.Where("id = ?", id).Delete(&Runtime{}).Error
}
