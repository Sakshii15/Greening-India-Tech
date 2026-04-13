package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	mw "github.com/taskflow/backend/internal/middleware"
	"github.com/taskflow/backend/internal/model"
)

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var exists bool
	if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	page, limit := getPagination(r)
	offset := (page - 1) * limit

	query := `SELECT id, title, description, status, priority, project_id, assignee_id, creator_id, due_date, created_at, updated_at
		FROM tasks WHERE project_id = $1`
	countQuery := `SELECT COUNT(*) FROM tasks WHERE project_id = $1`
	args := []interface{}{projectID}
	countArgs := []interface{}{projectID}
	argIdx := 2

	if status := r.URL.Query().Get("status"); status != "" {
		valid := map[string]bool{"todo": true, "in_progress": true, "done": true}
		if !valid[status] {
			writeValidationError(w, map[string]string{"status": "must be todo, in_progress, or done"})
			return
		}
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, status)
		countArgs = append(countArgs, status)
		argIdx++
	}

	if assignee := r.URL.Query().Get("assignee"); assignee != "" {
		query += fmt.Sprintf(` AND assignee_id = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND assignee_id = $%d`, argIdx)
		args = append(args, assignee)
		countArgs = append(countArgs, assignee)
		argIdx++
	}

	var totalCount int
	if err := h.db.QueryRow(countQuery, countArgs...).Scan(&totalCount); err != nil {
		slog.Error("count tasks error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		slog.Error("list tasks error", "error", err)
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

	writeJSON(w, http.StatusOK, model.PaginatedResponse{
		Data:       tasks,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
	})
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)
	projectID := chi.URLParam(r, "id")

	var exists bool
	if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var req model.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := make(map[string]string)
	if strings.TrimSpace(req.Title) == "" {
		fields["title"] = "is required"
	}
	if req.Priority != nil {
		valid := map[string]bool{"low": true, "medium": true, "high": true}
		if !valid[*req.Priority] {
			fields["priority"] = "must be low, medium, or high"
		}
	}
	if len(fields) > 0 {
		writeValidationError(w, fields)
		return
	}

	priority := "medium"
	if req.Priority != nil {
		priority = *req.Priority
	}

	var t model.Task
	err := h.db.QueryRow(
		`INSERT INTO tasks (title, description, priority, project_id, assignee_id, creator_id, due_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, title, description, status, priority, project_id, assignee_id, creator_id, due_date, created_at, updated_at`,
		strings.TrimSpace(req.Title), req.Description, priority, projectID, req.AssigneeID, userID, req.DueDate,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.ProjectID, &t.AssigneeID, &t.CreatorID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		slog.Error("create task error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)
	taskID := chi.URLParam(r, "id")

	var creatorID, projectID string
	err := h.db.QueryRow(
		`SELECT creator_id, project_id FROM tasks WHERE id = $1`, taskID,
	).Scan(&creatorID, &projectID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("update task lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var ownerID string
	h.db.QueryRow(`SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)

	if creatorID != userID && ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req model.UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := make(map[string]string)
	if req.Status != nil {
		valid := map[string]bool{"todo": true, "in_progress": true, "done": true}
		if !valid[*req.Status] {
			fields["status"] = "must be todo, in_progress, or done"
		}
	}
	if req.Priority != nil {
		valid := map[string]bool{"low": true, "medium": true, "high": true}
		if !valid[*req.Priority] {
			fields["priority"] = "must be low, medium, or high"
		}
	}
	if len(fields) > 0 {
		writeValidationError(w, fields)
		return
	}

	var t model.Task
	err := h.db.QueryRow(
		`UPDATE tasks SET
			title = COALESCE($1, title),
			description = COALESCE($2, description),
			status = COALESCE($3::task_status, status),
			priority = COALESCE($4::task_priority, priority),
			assignee_id = COALESCE($5::uuid, assignee_id),
			due_date = COALESCE($6::date, due_date),
			updated_at = now()
		 WHERE id = $7
		 RETURNING id, title, description, status, priority, project_id, assignee_id, creator_id, due_date, created_at, updated_at`,
		req.Title, req.Description, req.Status, req.Priority, req.AssigneeID, req.DueDate, taskID,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.ProjectID, &t.AssigneeID, &t.CreatorID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("update task error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(mw.UserIDKey).(string)
	taskID := chi.URLParam(r, "id")

	var creatorID, projectID string
	err := h.db.QueryRow(
		`SELECT creator_id, project_id FROM tasks WHERE id = $1`, taskID,
	).Scan(&creatorID, &projectID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("delete task lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var ownerID string
	h.db.QueryRow(`SELECT owner_id FROM projects WHERE id = $1`, projectID).Scan(&ownerID)

	if creatorID != userID && ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err = h.db.Exec(`DELETE FROM tasks WHERE id = $1`, taskID)
	if err != nil {
		slog.Error("delete task error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ProjectStats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var exists bool
	if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	byStatus := map[string]int{"todo": 0, "in_progress": 0, "done": 0}
	rows, err := h.db.Query(
		`SELECT status, COUNT(*) FROM tasks WHERE project_id = $1 GROUP BY status`, projectID,
	)
	if err != nil {
		slog.Error("stats by status error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		byStatus[status] = count
	}

	byAssignee := map[string]map[string]int{}
	rows2, err := h.db.Query(
		`SELECT COALESCE(u.name, 'unassigned'), t.status, COUNT(*)
		 FROM tasks t LEFT JOIN users u ON t.assignee_id = u.id
		 WHERE t.project_id = $1
		 GROUP BY u.name, t.status`, projectID,
	)
	if err != nil {
		slog.Error("stats by assignee error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var name, status string
		var count int
		rows2.Scan(&name, &status, &count)
		if byAssignee[name] == nil {
			byAssignee[name] = map[string]int{}
		}
		byAssignee[name][status] = count
	}

	writeJSON(w, http.StatusOK, model.ProjectStats{
		ByStatus:   byStatus,
		ByAssignee: byAssignee,
	})
}
