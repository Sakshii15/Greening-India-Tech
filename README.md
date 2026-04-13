# TaskFlow

Backend for a task management system — register, login, manage projects and tasks with role-based permissions.

## Stack

- Go 1.22 with chi router
- PostgreSQL 16
- JWT auth (golang-jwt/v5), bcrypt cost 12
- golang-migrate for schema migrations
- Structured logging via `log/slog`
- Docker multi-stage build

## Architecture

Went with a standard `cmd/` + `internal/` layout. Handlers are split by domain (auth, project, task) — nothing fancy, just keeps things easy to navigate.

I'm using raw `database/sql` instead of an ORM. For something this size, an ORM adds more complexity than it removes. I want to see exactly what queries are running.

For PATCH endpoints, I use `COALESCE($1, column)` so only the fields you send get updated. Downside: you can't explicitly null out a field. Acceptable tradeoff here.

I added a `creator_id` to tasks that wasn't in the original spec — needed it to properly check "only the task creator or project owner can delete a task."

### Things I skipped intentionally

- No rate limiting (would definitely add for prod, especially on `/auth/*`)
- Single JWT with 24h expiry, no refresh tokens — keeps it simple but not ideal long-term
- Any authenticated user can create tasks in any project they can see. A real app would need project membership.
- The stats endpoint runs two queries. Could be one, but readability > micro-optimization at this scale.

## Running Locally

```bash
git clone <repo-url>
cd taskflow
cp .env.example .env
docker compose up --build
```

API runs at `http://localhost:8080`. Migrations and seed data run automatically on startup.

## Test Credentials

Created by the seed script on first boot:
```
Email:    test@example.com
Password: password123
```

## API

Postman collection in `postman/TaskFlow.postman_collection.json` — import it, hit Login first, token auto-fills everywhere else.

### Auth

| Method | Endpoint | What it does |
|--------|----------|-------------|
| POST | `/auth/register` | Create account |
| POST | `/auth/login` | Get JWT token |

```bash
# login
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'
```

### Projects

All require `Authorization: Bearer <token>`.

| Method | Endpoint | What it does |
|--------|----------|-------------|
| GET | `/projects` | Your projects (paginated) |
| POST | `/projects` | New project |
| GET | `/projects/:id` | Project + its tasks |
| PATCH | `/projects/:id` | Edit (owner only) |
| DELETE | `/projects/:id` | Delete cascade (owner only) |
| GET | `/projects/:id/stats` | Task counts by status/assignee |

### Tasks

| Method | Endpoint | What it does |
|--------|----------|-------------|
| GET | `/projects/:id/tasks` | List (filterable, paginated) |
| POST | `/projects/:id/tasks` | New task |
| PATCH | `/tasks/:id` | Update fields |
| DELETE | `/tasks/:id` | Delete (creator or project owner) |

Filters: `?status=todo|in_progress|done`, `?assignee=<uuid>`
Pagination: `?page=1&limit=20`

### Errors

```json
{"error":"validation failed","fields":{"email":"is required"}}  // 400
{"error":"unauthorized"}                                         // 401
{"error":"forbidden"}                                            // 403
{"error":"not found"}                                            // 404
```

## Bonus stuff

- Pagination with total counts on list endpoints
- `/projects/:id/stats` for task breakdown
- Integration tests covering auth flows (register, login, validation, 401 on protected routes)

## What I'd improve

Refresh token rotation is the big one — a single long-lived JWT isn't great. I'd also add rate limiting on auth endpoints, wire the chi request ID into all log lines, and set up proper connection pool tuning (`MaxOpenConns`, etc). An `/health` endpoint for container orchestration would be easy to add. And honestly, project-level membership/invites would make the permissions model way more useful.
