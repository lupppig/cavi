package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/cavi/apps/api/internal/project"
)

// ProjectService is the project behaviour the handlers need. Every method
// authorises the actor itself.
type ProjectService interface {
	Create(ctx context.Context, actorID, orgID, workspaceID, name, description string) (project.Project, error)
	List(ctx context.Context, actorID, orgID string, opts project.ListOptions) (project.Page, error)
	Get(ctx context.Context, actorID, orgID, projectID string) (project.Project, error)
	Update(ctx context.Context, actorID, orgID, projectID string, ch project.Changes) (project.Project, error)
	Delete(ctx context.Context, actorID, orgID, projectID string) error
	Access(ctx context.Context, actorID, orgID, projectID string) ([]project.Grant, error)
	GrantAccess(ctx context.Context, actorID, orgID, projectID, userID string) error
	RevokeAccess(ctx context.Context, actorID, orgID, projectID, userID string) error
}

type projectHandler struct {
	svc ProjectService
	orgHandler
}

type workspaceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type userRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type projectResponse struct {
	ID             string       `json:"id"`
	OrganisationID string       `json:"organisation_id"`
	Workspace      workspaceRef `json:"workspace"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	CreatedBy      *userRef     `json:"created_by"`
	CreatedAt      string       `json:"created_at"`
	UpdatedAt      string       `json:"updated_at"`
	Permissions    []string     `json:"permissions"`
}

type projectPage struct {
	Data       []projectResponse `json:"data"`
	NextCursor *string           `json:"next_cursor"`
}

type grantResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url"`
	GrantedAt string `json:"granted_at"`
}

func toProjectResponse(p project.Project) projectResponse {
	perms := make([]string, len(p.Permissions))
	for i, x := range p.Permissions {
		perms[i] = string(x)
	}
	var by *userRef
	if p.CreatedByID != "" {
		by = &userRef{ID: p.CreatedByID, Name: p.CreatedByName}
	}
	return projectResponse{
		ID: p.ID, OrganisationID: p.OrganisationID,
		Workspace: workspaceRef{ID: p.WorkspaceID, Name: p.WorkspaceName},
		Name:      p.Name, Description: p.Description, CreatedBy: by,
		CreatedAt: ts(p.CreatedAt), UpdatedAt: ts(p.UpdatedAt), Permissions: perms,
	}
}

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	WorkspaceID string `json:"workspace_id"`
}

func (h projectHandler) create(c *gin.Context) {
	var req createProjectRequest
	if !decodeJSON(c, &req) {
		return
	}
	p, err := h.svc.Create(c.Request.Context(), actor(c), c.Param("orgID"), req.WorkspaceID, req.Name, req.Description)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, toProjectResponse(p))
}

func (h projectHandler) list(c *gin.Context) {
	opts := project.ListOptions{WorkspaceID: c.Query("workspace_id"), Cursor: c.Query("cursor")}
	if l := c.Query("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			n = -1 // rejected by the service with a field error
		}
		opts.Limit = n
	}
	page, err := h.svc.List(c.Request.Context(), actor(c), c.Param("orgID"), opts)
	if err != nil {
		h.fail(c, err)
		return
	}
	out := projectPage{Data: make([]projectResponse, len(page.Projects))}
	for i, p := range page.Projects {
		out.Data[i] = toProjectResponse(p)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, out)
}

func (h projectHandler) get(c *gin.Context) {
	p, err := h.svc.Get(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toProjectResponse(p))
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h projectHandler) update(c *gin.Context) {
	var req updateProjectRequest
	if !decodeJSON(c, &req) {
		return
	}
	p, err := h.svc.Update(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"),
		project.Changes{Name: req.Name, Description: req.Description})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toProjectResponse(p))
}

func (h projectHandler) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h projectHandler) access(c *gin.Context) {
	gs, err := h.svc.Access(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"))
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]grantResponse, len(gs))
	for i, g := range gs {
		out[i] = grantResponse{UserID: g.UserID, Email: g.Email, FirstName: g.FirstName, LastName: g.LastName,
			AvatarURL: g.AvatarURL, GrantedAt: ts(g.GrantedAt)}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, listResponse[grantResponse]{Data: out})
}

type grantRequest struct {
	UserID string `json:"user_id"`
}

func (h projectHandler) grant(c *gin.Context) {
	var req grantRequest
	if !decodeJSON(c, &req) {
		return
	}
	if err := h.svc.GrantAccess(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), req.UserID); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h projectHandler) revoke(c *gin.Context) {
	if err := h.svc.RevokeAccess(c.Request.Context(), actor(c), c.Param("orgID"), c.Param("projectID"), c.Param("userID")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
