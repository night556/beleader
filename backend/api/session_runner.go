package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"beleader/backend/db"
	"beleader/backend/session"
	"beleader/backend/tools"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
)

// RunSessionOpts controls how runSession builds prompts and selects tools.
type RunSessionOpts struct {
	AgentType     string   // "main", "coordinator", "worker", or "simple"
	RoleLabel     string   // label for messages in the UI
	CustomPrompt  string   // Worker/Simple/Tool/Skill: custom role definition (from agents table or Coordinator)
	EnableBrowser bool     // Worker (legacy): enable browser automation tools
	EnableDesktop bool     // Worker (legacy): enable desktop automation tools
	ToolNames     []string // Tool/Skill: tool names to register from global Registry
}

// runSession is the unified session runner.
func (h *Handler) runSession(sessionID, refID, workDir, userMessage string, opts RunSessionOpts) {
	ctx, cancel := context.WithCancel(context.Background())
	if opts.RoleLabel != "" {
		ctx = context.WithValue(ctx, session.CtxKeyRoleLabel, opts.RoleLabel)
	}
	if workDir != "" {
		ctx = context.WithValue(ctx, session.CtxKeyWorkDir, workDir)
	}
	pauseCh := make(chan struct{})
	interveneCh := make(chan session.InterveneMsg, 1)

	h.pauseMu.Lock()
	h.pauseChs[sessionID] = pauseCh
	h.interveneChs[sessionID] = interveneCh
	h.cancelFuncs[sessionID] = cancel
	h.pauseMu.Unlock()

	defer func() {
		h.pauseMu.Lock()
		delete(h.pauseChs, sessionID)
		delete(h.interveneChs, sessionID)
		delete(h.cancelFuncs, sessionID)
		h.pauseMu.Unlock()
	}()

	// ── Build system prompt ──
	sysPrompt := strings.Replace(session.CoreRules, "{work_dir}", workDir, 1)
	sysPrompt += envInfo(workDir)

	switch opts.AgentType {
	case "main":
		sysPrompt += "\n\n" + session.MainPrompt
	case "coordinator":
		prompt := session.CoordinatorPrompt
		if opts.CustomPrompt != "" {
			prompt = opts.CustomPrompt
		}
		sysPrompt += "\n\n" + strings.Replace(prompt, "{work_dir}", workDir, 1)
	case "worker":
		sysPrompt += "\n\n" + session.WorkerBasePrompt
		if opts.CustomPrompt != "" {
			sysPrompt += "\n\n## Role\n" + opts.CustomPrompt
		}
		if opts.EnableDesktop {
			sysPrompt += "\n\n" + session.DesktopRules
		}
		if opts.EnableBrowser {
			sysPrompt += "\n\n" + session.BrowserRules
		}
	case "simple":
		if opts.CustomPrompt != "" {
			sysPrompt += "\n\n" + opts.CustomPrompt
		}
	}

	h.Notify(SessionEvent{Type: "thinking", SessionID: sessionID})

	tools.BrowserHeadless = h.Config.Browser.Headless
	tools.BrowserProfileDir = h.Config.BrowserProfileDir()
	model := h.resolveModel(sessionID)
	llmClient := h.getClient(model.ID)

	// ── Per-session Manager for isolation ──
	sessionMgr := h.SessionMgr // main uses the handler's shared Manager
	if opts.AgentType != "main" {
		sessionMgr = session.NewManager(h.SessionMgr.DB, h.SessionMgr.LLM, h.SessionMgr.Config)
		if opts.AgentType == "coordinator" {
			registerAgentTools(sessionMgr, h.DB)
		}
	}

	// ── Register tools and build tool list ──
	var toolList []openai.Tool
	switch opts.AgentType {
	case "main":
		tools.RegisterKnowledgeTools(sessionMgr,
			func(query string, limit int) (string, error) {
				knowledge, err := h.DB.SearchKnowledge(query, limit)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(knowledge)
				return string(b), nil
			},
			func(title, content string) (int64, error) {
				return h.DB.InsertKnowledge(title, content, "main")
			},
			func(id int64) error {
				return h.DB.DeleteKnowledge(id)
			},
		)
		toolList = tools.MainTools(model.Vision)
	case "coordinator":
		tools.RegisterAll(sessionMgr, workDir, func(title, prompt string) (string, string, error) {
			return h.CreateProject(title, prompt)
		})
		tools.RegisterCoordinatorTools(
			sessionMgr,
			func(agentName, name, task string, enableBrowser, enableDesktop bool) (string, error) {
				return h.spawnWorker(sessionID, refID, agentName, name, task, enableBrowser, enableDesktop)
			},
			func(workerName string) (string, error) {
				return h.terminateWorker(refID, workerName)
			},
			func(workerName string) (string, error) {
				return h.deleteWorker(refID, workerName)
			},
			func(workerName, message string) (string, error) {
				return h.interveneWorker(refID, workerName, message)
			},
			func() (string, error) {
				return h.listWorkers(refID)
			},
		)
		tools.RegisterHTMLTools(sessionMgr)
		tools.RegisterKnowledgeTools(sessionMgr,
			func(query string, limit int) (string, error) {
				knowledge, err := h.DB.SearchKnowledge(query, limit)
				if err != nil {
					return "", err
				}
				b, _ := json.Marshal(knowledge)
				return string(b), nil
			},
			func(title, content string) (int64, error) {
				return h.DB.InsertKnowledge(title, content, refID)
			},
			func(id int64) error {
				return h.DB.DeleteKnowledge(id)
			},
		)
		toolList = tools.CoordinatorTools(model.Vision)
	case "worker":
		if len(opts.ToolNames) > 0 {
			tools.Global.RegisterTo(sessionMgr, opts.ToolNames)
			toolList = tools.Global.BuildToolList(opts.ToolNames)
		}
		if opts.EnableBrowser {
			tools.RegisterBrowserTools(sessionMgr)
			toolList = append(toolList, tools.BrowserTools()...)
		}
		if opts.EnableDesktop {
			tools.RegisterDesktopTools(sessionMgr)
			toolList = append(toolList, tools.DesktopTools()...)
		}
	case "simple":
		tools.RegisterReadFile(sessionMgr)
		tools.RegisterWriteFile(sessionMgr)
		toolList = append(toolList, tools.ReadFileToolDef())
		toolList = append(toolList, tools.WriteFileToolDef())
	}

	result, err := sessionMgr.RunLoop(ctx, sessionID, sysPrompt, userMessage, toolList, llmClient, model.ContextLimit, model.Vision, pauseCh, interveneCh,
		func(eventType string, payload map[string]any) {
			sid, _ := payload["session_id"].(string)
			h.Notify(SessionEvent{Type: eventType, SessionID: sid, Data: payload})
		})
	if err != nil {
		h.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "error", Content: err.Error()})
		h.Notify(SessionEvent{Type: "error", SessionID: sessionID, Data: gin.H{"message": err.Error()}})
		if opts.AgentType != "main" {
			h.releaseHC(sessionID)
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")
		}
		return
	}

	if result.Paused {
		h.Notify(SessionEvent{Type: "idle", SessionID: sessionID, Data: gin.H{"status": "idle", "session_id": sessionID}})
		if opts.AgentType != "main" {
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")
		}
		return
	}

	if result.Stopped {
		h.Notify(SessionEvent{Type: "idle", SessionID: sessionID, Data: gin.H{"status": "idle", "session_id": sessionID}})
		if opts.AgentType != "main" {
			h.releaseHC(sessionID)
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")
		}
		return
	}

	if result.Error != "" {
		h.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "error", Content: result.Error})
		h.Notify(SessionEvent{Type: "error", SessionID: sessionID, Data: gin.H{"message": result.Error}})
		if opts.AgentType == "main" {
			h.DB.UpdateSessionStatus(sessionID, "idle")
		} else {
			h.releaseHC(sessionID)
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")
		}
		return
	}

	if result.Completed {
		switch opts.AgentType {
		case "main":
			h.DB.UpdateSessionStatus("main", "idle")

		case "coordinator":
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.releaseHC(sessionID)
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")
			h.DB.UpdateProjectStatus(refID, "completed")
			h.Notify(SessionEvent{Type: "project_completed", SessionID: sessionID, Data: gin.H{"ref_id": refID, "status": "completed"}})

		case "worker":
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.releaseHC(sessionID)
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")

			// Auto-inject Worker's final response into Coordinator session
			agent, _ := h.getWorkerBySessionID(sessionID)
			if agent != nil {
				ref, err := h.DB.GetProject(agent.ProjectID)
				if err == nil {
					coordSid := h.getCoordinatorSessionID(ref)
					if coordSid != "" {
						workerMsg := fmt.Sprintf("[%s] 已完成\n\n%s\n\n---\nRemember: update STATUS.md with the results using edit_file.", agent.Name, result.Content)
						h.DB.InsertMessage(&db.Message{SessionID: coordSid, Role: "user", Content: workerMsg})

						// Notify Coordinator
						h.Notify(SessionEvent{
							Type:      "worker_completed",
							SessionID: coordSid,
							Data:      gin.H{"ref_id": agent.ProjectID, "worker_name": agent.Name, "worker_session_id": sessionID},
						})

						// If Coordinator is idle, start its RunLoop to process the result
						h.pauseMu.Lock()
						_, coordRunning := h.pauseChs[coordSid]
						h.pauseMu.Unlock()
						if !coordRunning {
							workDir := h.Config.ProjectDir(agent.ProjectID)
							go h.runSession(coordSid, agent.ProjectID, workDir, "", RunSessionOpts{
								AgentType: "coordinator",
								RoleLabel: "coordinator",
							})
						}
					}
				}
			}

		case "simple":
			h.DB.UpdateSessionStatus(sessionID, "idle")
			h.releaseHC(sessionID)
			h.DB.UpdateProjectAgentStatus("", sessionID, "idle")
		}

		h.Notify(SessionEvent{Type: "idle", SessionID: sessionID, Data: gin.H{"status": "idle", "session_id": sessionID}})
	}
}
