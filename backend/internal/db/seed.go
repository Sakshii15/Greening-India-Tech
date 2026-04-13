package db

import (
	"database/sql"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
)

func RunSeed(conn *sql.DB) error {
	var exists bool
	err := conn.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", "test@example.com").Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		slog.Info("seed data already exists, skipping")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	if err != nil {
		return err
	}

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRow(
		`INSERT INTO users (name, email, password) VALUES ($1, $2, $3) RETURNING id`,
		"Test User", "test@example.com", string(hash),
	).Scan(&userID)
	if err != nil {
		return err
	}

	var projectID string
	err = tx.QueryRow(
		`INSERT INTO projects (name, description, owner_id) VALUES ($1, $2, $3) RETURNING id`,
		"Website Redesign", "Q2 redesign project", userID,
	).Scan(&projectID)
	if err != nil {
		return err
	}

	tasks := []struct {
		title    string
		status   string
		priority string
	}{
		{"Design homepage mockup", "todo", "high"},
		{"Implement auth flow", "in_progress", "high"},
		{"Write API documentation", "done", "medium"},
	}

	for _, t := range tasks {
		_, err = tx.Exec(
			`INSERT INTO tasks (title, status, priority, project_id, assignee_id, creator_id)
			 VALUES ($1, $2, $3, $4, $5, $5)`,
			t.title, t.status, t.priority, projectID, userID,
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("seed data inserted successfully")
	return nil
}
