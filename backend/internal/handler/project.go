package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	mw "github.com/taskflow/backend/internal/middleware"
	"github.com/taskflow/backend/internal/model"
)

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)
	page, limit := getPagination(r)
	offset := (page - 1) * limit

	var totalCount int
	err := h.db.QueryRow(
		`SELECT COUNT(DISTINCT p.id) FROM projects p
		 LEFT JOIN tasks t ON t.project_id = p.id
		 WHERE p.owner_id = $1 OR t.assignee_id = $1`, userID,
	).Scan(&totalCount)
	if err != nil {
		slog.Error("count projects error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := h.db.Query(
		`SELECT DISTINCT p.id, p.name, p.description, p.owner_id, p.created_at
		 FROM projects p
		 LEFT JOIN tasks t ON t.project_id = p.id
		 WHERE p.owner_id = $1 OR t.assignee_id = $1
		 ORDER BY p.created_at DESC
		 LIMIT $2 OFFSET $3`, userID, limit, offset,
	)
	if err != nil {
		slog.Error("list projects error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	projects := []model.Project{}
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt); err != nil {
			slog.Error("scan project error", "error", err)
			continue
		}
		projects = append(projects, p)
	}

	writeJSON(w, http.StatusOK, model.PaginatedResponse{
		Data:       projects,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
	})
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)

	var req model.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeValidationError(w, map[string]string{"name": "is required"})
		return
	}

	var p model.Project
	err := h.db.QueryRow(
		`INSERT INTO projects (name, description, owner_id) VALUES ($1, $2, $3)
		 RETURNING id, name, description, owner_id, created_at`,
		strings.TrimSpace(req.Name), req.Description, userID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt)
	if err != nil {
		slog.Error("create project error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var p model.Project
	err := h.db.QueryRow(
		`SELECT id, name, description, owner_id, created_at FROM projects WHERE id = $1`,
		projectID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("get project error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := h.db.Query(
		`SELECT id, title, description, status, priority, project_id, assignee_id, creator_id, due_date, created_at, updated_at
		 FROM tasks WHERE project_id = $1 ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		slog.Error("get project tasks error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	tasks := []model.Task{}
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority,
			&t.ProjectID, &t.AssigneeID, &t.CreatorID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			slog.Error("scan task error", "error", err)
			continue
		}
		tasks = append(tasks, t)
	}

	writeJSON(w, http.StatusOK, model.ProjectWithTasks{Project: p, Tasks: tasks})
}

func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)
	projectID := chi.URLParam(r, "id")

	var ownerID string
	err := h.db.QueryRow(`SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("update project lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req model.UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var p model.Project
	err = h.db.QueryRow(
		`UPDATE projects SET
			name = COALESCE($1, name),
			description = COALESCE($2, description)
		 WHERE id = $3
		 RETURNING id, name, description, owner_id, created_at`,
		req.Name, req.Description, projectID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt)
	if err != nil {
		slog.Error("update project error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)
	projectID := chi.URLParam(r, "id")

	var ownerID string
	err := h.db.QueryRow(`SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("delete project lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err = h.db.Exec(`DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		slog.Error("delete project error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
