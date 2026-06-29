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

	if schemaVersion < 2 {
		db.GORM.Exec("ALTER TABLE agents ADD COLUMN type TEXT DEFAULT ''")
		db.GORM.Exec("ALTER TABLE agents ADD COLUMN tools TEXT DEFAULT '[]'")
		db.GORM.Exec("ALTER TABLE agents ADD COLUMN tool_agents TEXT DEFAULT '[]'")
		db.GORM.Exec("PRAGMA user_version = 2")
	}

	// Seed system tool agents
	db.seedToolAgents()

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
	TotalTokens     int       `gorm:"column:total_tokens;default:0" json:"total_tokens"`
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
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name       string    `gorm:"size:128;uniqueIndex" json:"name"`
	Desc       string    `gorm:"size:512;default:''" json:"desc"`
	Type       string    `gorm:"size:32;default:''" json:"type"`               // "" | "tool_agent" | "skill_agent"
	Tools      string    `gorm:"type:text;default:'[]'" json:"tools"`          // JSON array of tool names
	ToolAgents string    `gorm:"type:text;default:'[]'" json:"tool_agents"`    // JSON array of tool agent names
	Content    string    `gorm:"default:''" json:"content"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
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

func (db *DB) UpdateSessionRounds(id string, rounds, contextPct, totalTokens int) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).Updates(map[string]any{
		"rounds":            rounds,
		"context_usage_pct": contextPct,
		"total_tokens":      totalTokens,
	}).Error
}

func (db *DB) GetSessionTokens(id string) (int, error) {
	var s Session
	err := db.GORM.Select("total_tokens").Where("id = ?", id).First(&s).Error
	if err != nil {
		return 0, err
	}
	return s.TotalTokens, nil
}

func (db *DB) GetProjectTotalTokens(projectID string) (int, error) {
	var total int
	err := db.GORM.Model(&Session{}).
		Joins("JOIN project_agents ON project_agents.session_id = sessions.id").
		Where("project_agents.project_id = ?", projectID).
		Select("COALESCE(SUM(sessions.total_tokens), 0)").
		Scan(&total).Error
	return total, err
}

func (db *DB) IncrementSessionTokens(id string, delta int) error {
	return db.GORM.Model(&Session{}).Where("id = ?", id).
		UpdateColumn("total_tokens", gorm.Expr("total_tokens + ?", delta)).Error
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

// GetMessagesSessionBubble 按用户气泡分页加载单 session 消息。
// beforeID=0 表示首屏（最新 turns 个气泡）；非 0 表示加载 id < beforeID 的更早 turns 个气泡。
// 一个气泡 = 一条 role='user' 消息 + 后续所有非 user 消息。
func (db *DB) GetMessagesSessionBubble(sessionID string, beforeID int64, turns int) ([]Message, error) {
	cutoffID, err := db.bubbleCutoff([]string{sessionID}, beforeID, turns)
	if err != nil {
		return nil, err
	}
	q := db.GORM.Model(&Message{}).Where("session_id = ?", sessionID)
	if cutoffID > 0 {
		q = q.Where("id >= ?", cutoffID)
	}
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var msgs []Message
	err = q.Order("id ASC").Find(&msgs).Error
	return msgs, err
}

// GetMessagesProjectBubble 按用户气泡分页加载 project 合并消息。
// coordinator session 取所有 role；worker session 跳过 role='user'（任务消息，已在 coordinator 的 spawn_worker 工具调用参数里）。
func (db *DB) GetMessagesProjectBubble(coordSids, workerSids []string, beforeID int64, turns int) ([]Message, error) {
	cutoffID, err := db.bubbleCutoff(coordSids, beforeID, turns)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	q := db.GORM.Model(&Message{})
	if len(coordSids) > 0 && len(workerSids) > 0 {
		q = q.Where("(session_id IN ?) OR (session_id IN ? AND role != 'user')", coordSids, workerSids)
	} else if len(coordSids) > 0 {
		q = q.Where("session_id IN ?", coordSids)
	} else if len(workerSids) > 0 {
		q = q.Where("session_id IN ? AND role != 'user'", workerSids)
	} else {
		return msgs, nil
	}
	if cutoffID > 0 {
		q = q.Where("id >= ?", cutoffID)
	}
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	err = q.Order("id ASC").Find(&msgs).Error
	return msgs, err
}

// bubbleCutoff 找到第 turns 个最老的 user 消息 id（作为气泡分页的左边界）。
// user 消息只在 coordinator session（worker 的被过滤）。返回 0 表示不足 turns 个，从头加载。
func (db *DB) bubbleCutoff(coordSids []string, beforeID int64, turns int) (int64, error) {
	if len(coordSids) == 0 || turns <= 0 {
		return 0, nil
	}
	q := db.GORM.Model(&Message{}).
		Where("session_id IN ? AND role = 'user'", coordSids).
		Order("id DESC").
		Limit(1).
		Offset(turns - 1)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var m Message
	err := q.First(&m).Error
	if err != nil {
		return 0, nil // 不足 turns 个，从头加载
	}
	return m.ID, nil
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

func (db *DB) GetAgentByID(id int64) (*Agent, error) {
	var a Agent
	if err := db.GORM.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (db *DB) UpdateAgentByID(id int64, name, content string) error {
	return db.GORM.Model(&Agent{}).Where("id = ?", id).Updates(map[string]any{
		"name":    name,
		"content": content,
	}).Error
}

func (db *DB) UpdateAgentDescByID(id int64, desc string) error {
	return db.GORM.Model(&Agent{}).Where("id = ?", id).Update("desc", desc).Error
}

func (db *DB) DeleteAgentByID(id int64) error {
	return db.GORM.Where("id = ?", id).Delete(&Agent{}).Error
}

func (db *DB) UpdateAgentByIDFull(id int64, name, desc, content, agentType, tools, toolAgents string) error {
	return db.GORM.Model(&Agent{}).Where("id = ?", id).Updates(map[string]any{
		"name":        name,
		"desc":        desc,
		"content":     content,
		"type":        agentType,
		"tools":       tools,
		"tool_agents": toolAgents,
	}).Error
}

func (db *DB) ListToolAgents() ([]Agent, error) {
	var agents []Agent
	err := db.GORM.Where("type = 'tool_agent'").Order("name ASC").Find(&agents).Error
	return agents, err
}

func (db *DB) seedToolAgents() {
	var count int64
	coordinatorTools := `["read_file","read_dir","write_file","edit_file","delete_file","search_content","search_files","read_status","write_status","run_command","run_http_request","web_search","web_fetch","spawn_worker","terminate_worker","delete_worker","intervene_worker","list_workers","list_agents","create_agent","edit_agent","delete_agent","show_html","close_html","list_htmls","focus_session","show_file","search_knowledge","save_knowledge","delete_knowledge","create_project"]`
	if db.GORM.Model(&Agent{}).Where("name = 'coordinator'").Count(&count); count == 0 {
		db.GORM.Create(&Agent{
			Name:    "coordinator",
			Desc:    "Project orchestrator — plan, delegate, manage workers and project state",
			Type:    "coordinator",
			Content: `You are the Coordinator of this project. You manage, plan, and orchestrate — you do not execute. Your value is in understanding what needs to be done, making good decisions, and delegating to the right Worker.

## STATUS.md maintenance
STATUS.md is the project's status entry point. It records current progress, completed and pending items, key decisions, and serves as a navigation hub pointing to the project's various documents and artifacts (requirements, design docs, technical specs, API designs, etc. — whatever the project needs, not a fixed checklist).

When to update: after every Worker completes, when the user gives new requirements, or when project state changes.

How to update:
1. If STATUS.md content is still fresh in context from a recent read or write, update from memory — don't waste tokens re-reading
2. If unsure of the current content, call read_status first
3. Use write_status to write the complete updated content
4. Organize naturally based on the actual project — a small project may be a brief progress list, a large project needs sections referencing various documents

Do NOT turn it into a log or journal. Do NOT repeat the same information. Do NOT discard important past records while updating.

## How to respond
First, judge the situation:
- **Casual chat / discussion** — the user just wants to talk. Be a conversational partner, reply naturally. Don't spawn Workers.
- **Question / advice** — the user wants to understand something. Answer directly. Use web_search or web_fetch if helpful.
- **Research** — the user needs information gathered before deciding. Either answer from your own knowledge or spawn a researcher Worker.
- **Development** — the user wants something built. Spawn a Worker.

If the task evolves (e.g. conversation turns into development), adapt accordingly.

## Development workflow
Spawn Workers one at a time or in small batches. Wait for each Worker to finish and report back. Do NOT call intervene_worker immediately after spawning — the Worker will respond when done or blocked.

A Worker that has finished still holds its conversation context. If a follow-up task is closely related to what that Worker already did, use intervene_worker to give it the new task instead of spawning a fresh Worker that would need to re-learn the context.`,
			Tools: coordinatorTools,
		})
	}
	if db.GORM.Model(&Agent{}).Where("type = 'tool_agent' AND name = 'browser'").Count(&count); count == 0 {
		db.GORM.Create(&Agent{
			Name:    "browser",
			Desc:    "Browser automation — navigate pages, click, type, scroll, screenshot",
			Type:    "tool_agent",
			Content: "You are a browser automation agent. You control a web browser to navigate pages, interact with UI elements, extract data, and take screenshots.\n\n## Before Acting\n- Inspect page state (browser_content) before any interaction.\n\n## After Each Action\n- Verify the result — compare browser_content before/after.\n- If page didn't change as expected, try a different approach.\n\n## Interaction\n- Prefer ref numbers from the latest snapshot. CSS selectors only as fallback.\n- After typing into a search box, press Enter with browser_keys.\n- For dropdowns, use browser_select to see options before choosing.\n\n## Extraction\n- Use browser_content for text. Only screenshot as last resort.\n- Use browser_evaluate for targeted data extraction.\n\n## Cleanup\n- Close open tabs you no longer need with browser_close.\n- If the result should be displayed to the user, keep that tab open.\n\nWhen done, summarize what you accomplished and any key findings.",
			Tools: `["browser_open","browser_close","browser_list","browser_switch","browser_click","browser_input","browser_scroll","browser_content","browser_evaluate","browser_screenshot","browser_sleep","browser_keys","browser_back","browser_select"]`,
		})
	}
	if db.GORM.Model(&Agent{}).Where("type = 'tool_agent' AND name = 'desktop'").Count(&count); count == 0 {
		db.GORM.Create(&Agent{
			Name:    "desktop",
			Desc:    "Desktop automation — screenshot, click, type, window management",
			Type:    "tool_agent",
			Content: "You are a desktop automation agent. You control the desktop through a screenshot-analyze-act loop.\n\n## Coordinate System\nAll coordinates use a normalized 0-1000 grid. (0,0)=top-left, (1000,1000)=bottom-right, (500,500)=center.\n\n## Strategy\n- Start with a screenshot to see the current screen state.\n- Before every click, state what you are targeting.\n- If the screen shows something unexpected, analyze and adapt.\n- After any action that changes the UI, take a screenshot to verify.\n- For text input, prefer desktop_type_text — it supports Chinese and Unicode.\n- For very small targets, keyboard shortcuts are more reliable than clicking.\n\nWhen done, summarize what you accomplished.",
			Tools: `["desktop_screenshot","desktop_click","desktop_double_click","desktop_move","desktop_drag","desktop_scroll","desktop_type_text","desktop_key_tap","desktop_clipboard_read","desktop_clipboard_write","desktop_window_list","desktop_window_activate","desktop_window_minimize","desktop_window_maximize","desktop_window_close","desktop_process_list","desktop_mouse_info","desktop_screen_info","desktop_active_window","desktop_sleep"]`,
		})
	}
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
