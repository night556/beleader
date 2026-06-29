package api

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateProject creates a project with an optional agent template.
// If agentName is empty or "coordinator", the project gets a Coordinator agent with full orchestration tools.
// Otherwise, the named agent template is used with only read_file + write_file tools.
func (h *Handler) CreateProject(title, prompt, agentName string) (string, string, error) {
	refID := uuid.New().String()
	coordinatorSessionID := uuid.New().String()
	workDir := h.Config.ProjectDir(refID)

	os.MkdirAll(workDir, 0755)

	// Create empty STATUS.md
	os.WriteFile(h.Config.StatusPath(refID), []byte("# "+title+"\n\n"), 0644)

	if title == "" {
		if prompt != "" {
			title = truncate(prompt, 50)
		} else {
			title = "Chat"
		}
	}

	// If no prompt, Coordinator acts as a conversational partner
	firstMessage := prompt
	if firstMessage == "" {
		firstMessage = "Hello! What would you like to talk about?"
	}

	h.DB.CreateProject(refID, title, workDir)

	// Determine agent type based on template selection
	agentType := "coordinator"
	roleLabel := "coordinator"
	projectAgentName := "coordinator"
	var customPrompt string

	if agentName != "" && agentName != "coordinator" {
		if agent, err := h.DB.GetAgentByName(agentName); err == nil {
			agentType = "simple"
			roleLabel = agent.Name
			projectAgentName = agent.Name
			customPrompt = agent.Content
		}
	}

	h.acquireHC(coordinatorSessionID)
	h.DB.CreateSession(coordinatorSessionID, "running")
	h.DB.AddProjectAgent(refID, projectAgentName, coordinatorSessionID, agentType, customPrompt, false, false)

	h.Notify(SessionEvent{
		Type: "project_created",
		Data: gin.H{
			"ref_id":     refID,
			"session_id": coordinatorSessionID,
			"title":      title,
			"status":     "running",
		},
	})

	go h.runSession(coordinatorSessionID, refID, workDir, firstMessage, RunSessionOpts{
		AgentType:    agentType,
		RoleLabel:    roleLabel,
		CustomPrompt: customPrompt,
	})

	return refID, coordinatorSessionID, nil
}

func (h *Handler) handleCreateProject(c *gin.Context) {
	var req struct {
		Title     string `json:"title"`
		Prompt    string `json:"prompt"`
		AgentName string `json:"agent_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	refID, _, err := h.CreateProject(req.Title, req.Prompt, req.AgentName)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ref_id": refID, "status": "ok"})
}
