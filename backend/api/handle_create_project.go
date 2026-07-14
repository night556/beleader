package api

import (
	"os"

	"beleader/backend/db"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateProject creates a project with a Coordinator as the project-level agent.
// When prompt is non-empty (AI-initiated via create_project tool), the coordinator
// starts immediately with that prompt as its first message.
// When prompt is empty (manual creation via UI), the coordinator is created idle —
// the user's first typed message triggers it via handleIntervene.
func (h *Handler) CreateProject(title, prompt string) (string, string, error) {
	refID := uuid.New().String()
	coordinatorSessionID := uuid.New().String()
	workDir := h.Config.ProjectDir(refID)

	os.MkdirAll(workDir, 0755)
	os.WriteFile(h.Config.StatusPath(refID), []byte("# "+title+"\n\n"), 0644)

	if title == "" {
		if prompt != "" {
			title = truncate(prompt, 50)
		} else {
			title = "Chat"
		}
	}

	h.DB.CreateProject(refID, title, workDir)

	h.DB.CreateSession(coordinatorSessionID, "idle")
	h.DB.AddProjectAgent(refID, "coordinator", coordinatorSessionID, "coordinator", "", "", false)

	h.Notify(SessionEvent{
		Type: "project_created",
		Data: gin.H{
			"ref_id":     refID,
			"session_id": coordinatorSessionID,
			"title":      title,
			"status":     "idle",
		},
	})

	if prompt != "" {
		h.acquireHC(coordinatorSessionID)
		h.DB.UpdateSessionStatus(coordinatorSessionID, "running")
		h.DB.UpdateProjectStatus(refID, "running")
		h.DB.InsertMessage(&db.Message{SessionID: coordinatorSessionID, Role: "user", Content: prompt})
		go h.runSession(coordinatorSessionID, refID, workDir, prompt, RunSessionOpts{
			AgentType: "coordinator",
			RoleLabel: "coordinator",
		})
	}

	return refID, coordinatorSessionID, nil
}

func (h *Handler) handleCreateProject(c *gin.Context) {
	var req struct {
		Title  string `json:"title"`
		Prompt string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	refID, _, err := h.CreateProject(req.Title, req.Prompt)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ref_id": refID, "status": "ok"})
}
