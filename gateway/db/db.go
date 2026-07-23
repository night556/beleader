package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	GORM   *gorm.DB
	Driver string // "sqlite" | "mysql" | "postgres"
}

type DBConfig struct {
	Driver   string
	Path     string // SQLite
	Host     string // MySQL/PG
	Port     int
	User     string
	Password string
	DBName   string
}

func Open(cfg DBConfig) (*DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
		dialector = mysql.Open(dsn)
	case "postgres":
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName)
		dialector = postgres.Open(dsn)
	default: // sqlite
		dialector = sqlite.Open(cfg.Path + "?_journal_mode=WAL&_foreign_keys=on")
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	if cfg.Driver == "sqlite" {
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(25)
	}

	db := &DB{GORM: gormDB, Driver: cfg.Driver}
	if err := db.autoMigrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func LoadDBConfig() DBConfig {
	driver := os.Getenv("DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	cfg := DBConfig{Driver: driver}
	switch driver {
	case "sqlite":
		cfg.Path = os.Getenv("DB_PATH")
		if cfg.Path == "" {
			cfg.Path = filepath.Join(ConfigDir(), "beleader.db")
		}
	case "mysql":
		cfg.Host = envOr("DB_HOST", "127.0.0.1")
		cfg.Port = envInt("DB_PORT", 3306)
		cfg.User = envOr("DB_USER", "beleader")
		cfg.Password = os.Getenv("DB_PASSWORD")
		cfg.DBName = envOr("DB_NAME", "beleader")
	case "postgres":
		cfg.Host = envOr("DB_HOST", "127.0.0.1")
		cfg.Port = envInt("DB_PORT", 5432)
		cfg.User = envOr("DB_USER", "beleader")
		cfg.Password = os.Getenv("DB_PASSWORD")
		cfg.DBName = envOr("DB_NAME", "beleader")
	}
	return cfg
}

func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".beleader")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	fmt.Sscanf(v, "%d", &n)
	if n == 0 {
		return def
	}
	return n
}

func (db *DB) Close() error {
	sqlDB, err := db.GORM.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ── Schema version ──

func (db *DB) schemaVersion() int {
	if db.Driver == "sqlite" {
		var v int
		db.GORM.Raw("PRAGMA user_version").Scan(&v)
		return v
	}
	db.GORM.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY)")
	var v int
	db.GORM.Raw("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&v)
	return v
}

func (db *DB) setSchemaVersion(v int) {
	if db.Driver == "sqlite" {
		db.GORM.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
	} else {
		db.GORM.Exec("INSERT INTO schema_migrations (version) VALUES (?) ON CONFLICT DO NOTHING", v)
	}
}

func (db *DB) autoMigrate() error {
	if err := db.GORM.AutoMigrate(
		&Pool{}, &ToolAgent{}, &Thread{}, &Message{}, &Event{},
		&Agent{}, &ModelProfile{}, &MCPServer{},
	); err != nil {
		return err
	}
	v := db.schemaVersion()
	if v < 1 {
		db.setSchemaVersion(1)
	}
	db.seedDefaultAgent()
	return nil
}

// ── Pool ──

// ── Pool methods ──

func (db *DB) ListPools() ([]Pool, error) {
	var pools []Pool
	err := db.GORM.Order("id ASC").Find(&pools).Error
	return pools, err
}

func (db *DB) GetPool(id int64) (*Pool, error) {
	var p Pool
	if err := db.GORM.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetPoolByName(name string) (*Pool, error) {
	var p Pool
	if err := db.GORM.Where("name = ?", name).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetDefaultPool() (*Pool, error) {
	var p Pool
	if err := db.GORM.Where("is_default = 1").First(&p).Error; err != nil {
		// fallback: first pool
		if err2 := db.GORM.First(&p).Error; err2 != nil {
			return nil, err2
		}
	}
	return &p, nil
}

func (db *DB) CreatePool(p *Pool) error {
	return db.GORM.Create(p).Error
}

func (db *DB) UpdatePool(p *Pool) error {
	return db.GORM.Save(p).Error
}

func (db *DB) DeletePool(id int64) error {
	return db.GORM.Where("id = ?", id).Delete(&Pool{}).Error
}

func (db *DB) UpdatePoolToolDefs(id int64, toolDefs string) error {
	return db.GORM.Model(&Pool{}).Where("id = ?", id).Update("tool_defs", toolDefs).Error
}

// ── ToolAgent methods ──

func (db *DB) UpsertToolAgent(name, url string, poolID int64) (*ToolAgent, error) {
	var ta ToolAgent
	err := db.GORM.Where("name = ?", name).First(&ta).Error
	if err != nil {
		ta = ToolAgent{Name: name, URL: url, PoolID: poolID, Status: "active", LastHeartbeat: time.Now()}
		if createErr := db.GORM.Create(&ta).Error; createErr != nil {
			return nil, createErr
		}
		return &ta, nil
	}
	ta.URL = url
	ta.PoolID = poolID
	ta.Status = "active"
	ta.LastHeartbeat = time.Now()
	if updateErr := db.GORM.Save(&ta).Error; updateErr != nil {
		return nil, updateErr
	}
	return &ta, nil
}

func (db *DB) UpdateToolAgentHeartbeat(id int64, status string) error {
	updates := map[string]any{"last_heartbeat": time.Now()}
	if status != "" {
		updates["status"] = status
	}
	return db.GORM.Model(&ToolAgent{}).Where("id = ?", id).Updates(updates).Error
}

func (db *DB) ListToolAgents() ([]ToolAgent, error) {
	var agents []ToolAgent
	err := db.GORM.Order("name ASC").Find(&agents).Error
	return agents, err
}

func (db *DB) ListActiveToolAgentsByPool(poolID int64) ([]ToolAgent, error) {
	var agents []ToolAgent
	err := db.GORM.Where("pool_id = ? AND status = ?", poolID, "active").Order("id ASC").Find(&agents).Error
	return agents, err
}

func (db *DB) GetToolAgent(id int64) (*ToolAgent, error) {
	var ta ToolAgent
	if err := db.GORM.First(&ta, id).Error; err != nil {
		return nil, err
	}
	return &ta, nil
}

func (db *DB) DeleteToolAgent(id int64) error {
	return db.GORM.Where("id = ?", id).Delete(&ToolAgent{}).Error
}

// ── Thread methods ──

func (db *DB) CreateThread(id, title string, agentID int64, modelID string, poolID int64, workspacePath string) error {
	return db.GORM.Create(&Thread{
		ID: id, Title: title, AgentID: agentID, ModelID: modelID,
		PoolID: poolID, WorkspacePath: workspacePath,
	}).Error
}

func (db *DB) CreateWorkerThread(id, title, parentThreadID string, agentID int64, modelID string, poolID int64, workspacePath string) error {
	return db.GORM.Create(&Thread{
		ID: id, Title: title, AgentID: agentID, ModelID: modelID,
		PoolID: poolID, WorkspacePath: workspacePath,
		ParentThreadID: parentThreadID, Status: "running",
	}).Error
}

func (db *DB) SetThreadStatus(id, status string) error {
	return db.GORM.Model(&Thread{}).Where("id = ?", id).Update("status", status).Error
}

func (db *DB) UpdateThread(id string, updates map[string]any) error {
	return db.GORM.Model(&Thread{}).Where("id = ?", id).Updates(updates).Error
}

func (db *DB) GetCompletedWorkers(parentThreadID string) ([]Thread, error) {
	var threads []Thread
	err := db.GORM.Where("parent_thread_id = ? AND status = 'completed' AND result_delivered = false", parentThreadID).Find(&threads).Error
	return threads, err
}

func (db *DB) MarkWorkersDelivered(ids []string) error {
	return db.GORM.Model(&Thread{}).Where("id IN ?", ids).Update("result_delivered", true).Error
}

func (db *DB) GetActiveWorkers(parentThreadID string) ([]Thread, error) {
	var threads []Thread
	err := db.GORM.Where("parent_thread_id = ? AND status = 'running'", parentThreadID).Find(&threads).Error
	return threads, err
}

func (db *DB) StopWorkers(parentThreadID string) error {
	return db.GORM.Model(&Thread{}).Where("parent_thread_id = ? AND status = 'running'", parentThreadID).Update("status", "stopped").Error
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
	err := db.GORM.Where("parent_thread_id = ''").Order("updated_at DESC").Find(&threads).Error
	return threads, err
}

func (db *DB) DeleteThread(id string) error {
	db.DeleteMessages(id)
	db.DeleteEvents(id)
	// delete child threads
	var children []Thread
	db.GORM.Where("parent_thread_id = ?", id).Find(&children)
	for _, c := range children {
		db.DeleteThread(c.ID)
	}
	return db.GORM.Where("id = ?", id).Delete(&Thread{}).Error
}

func (db *DB) UpdateThreadTitle(id, title string) error {
	return db.GORM.Model(&Thread{}).Where("id = ?", id).Updates(map[string]any{
		"title":      title,
		"updated_at": time.Now(),
	}).Error
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

// GetRecentMessagesByCount returns the last N messages for a thread.
func (db *DB) GetRecentMessagesByCount(threadID string, limit int) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("thread_id = ?", threadID).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (db *DB) GetMessagesByIDs(threadID string, ids []int64) ([]Message, error) {
	var msgs []Message
	err := db.GORM.Where("thread_id = ? AND id IN ?", threadID, ids).
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

// ── Event methods ──

func (db *DB) InsertEvent(e *Event) (int64, error) {
	if err := db.GORM.Create(e).Error; err != nil {
		return 0, err
	}
	return e.ID, nil
}

func (db *DB) GetEvents(threadID string, sinceID int64) ([]Event, error) {
	var events []Event
	err := db.GORM.Where("thread_id = ? AND id > ?", threadID, sinceID).
		Order("id ASC").
		Find(&events).Error
	return events, err
}

func (db *DB) DeleteEvents(threadID string) error {
	return db.GORM.Where("thread_id = ?", threadID).Delete(&Event{}).Error
}

// ── Agent methods ──

func (db *DB) CreateAgent(name, desc, systemPrompt, tools, defaultModelID, mcpServers, workerAgents string) error {
	return db.GORM.Create(&Agent{
		Name: name, Desc: desc, SystemPrompt: systemPrompt, Tools: tools,
		DefaultModelID: defaultModelID, MCPServers: mcpServers, WorkerAgents: workerAgents,
	}).Error
}

func (db *DB) UpdateAgent(id int64, name, desc, systemPrompt, tools, defaultModelID, mcpServers, workerAgents string) error {
	return db.GORM.Model(&Agent{}).Where("id = ?", id).Updates(map[string]any{
		"name": name, "desc": desc, "system_prompt": systemPrompt,
		"tools": tools, "default_model_id": defaultModelID,
		"mcp_servers": mcpServers, "worker_agents": workerAgents,
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

func (db *DB) GetAgentByName(name string) (*Agent, error) {
	var a Agent
	if err := db.GORM.Where("name = ?", name).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ── Model methods ──

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

func (db *DB) CreateModel(m *ModelProfile) error {
	m.ID = 0
	m.IsActive = false
	return db.GORM.Create(m).Error
}

func (db *DB) UpdateModel(modelID string, m *ModelProfile) error {
	return db.GORM.Model(&ModelProfile{}).Where("model_id = ?", modelID).Updates(map[string]any{
		"base_url": m.BaseURL, "api_key": m.APIKey, "model": m.Model,
		"vision": m.Vision, "context_limit": m.ContextLimit, "reasoning_effort": m.ReasoningEffort,
	}).Error
}

func (db *DB) DeleteModel(modelID string) error {
	return db.GORM.Where("model_id = ?", modelID).Delete(&ModelProfile{}).Error
}

func (db *DB) SetActiveModel(modelID string) error {
	return db.GORM.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ModelProfile{}).Where("1=1").Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Model(&ModelProfile{}).Where("model_id = ?", modelID).Update("is_active", true).Error
	})
}

// ── MCP Server methods ──

func (db *DB) CreateMCPServer(s *MCPServer) error {
	return db.GORM.Create(s).Error
}

func (db *DB) UpdateMCPServer(s *MCPServer) error {
	return db.GORM.Model(&MCPServer{}).Where("id = ?", s.ID).Updates(map[string]any{
		"name": s.Name, "type": s.Type, "enabled": s.Enabled,
		"command": s.Command, "args": s.Args, "env": s.Env,
		"url": s.URL, "headers": s.Headers, "status": s.Status, "error": s.Error,
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

// ── Helpers ──

func ParsePinnedIDs(s string) []int64 {
	var ids []int64
	json.Unmarshal([]byte(s), &ids)
	return ids
}

func MarshalPinnedIDs(ids []int64) string {
	b, _ := json.Marshal(ids)
	return string(b)
}
