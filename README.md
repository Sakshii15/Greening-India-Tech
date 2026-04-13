# TaskFlow

A RESTful backend for a task management system built with Go. Supports user registration and authentication, project management, and task tracking with role-based permissions.

## Tech Stack

- **Go 1.22** with [chi](https://github.com/go-chi/chi) router
- **PostgreSQL 16** with UUID primary keys and enum types
- **JWT authentication** (golang-jwt/v5) with bcrypt password hashing (cost 12)
- **golang-migrate** for versioned schema migrations
- **Structured logging** via `log/slog` (JSON output)
- **Docker** multi-stage build with Compose orchestration

## Project Structure

```
backend/
├── cmd/server/main.go          # Entry point, routing, graceful shutdown
├── internal/
│   ├── db/
│   │   ├── migrate.go          # Migration runner
│   │   └── seed.go             # Seed data (test user, sample project/tasks)
│   ├── handler/
│   │   ├── handler.go          # Shared handler struct, helpers
│   │   ├── auth.go             # Register, Login, JWT generation
│   │   ├── project.go          # Project CRUD
│   │   ├── task.go             # Task CRUD + project stats
│   │   └── auth_test.go        # Integration tests for auth flows
│   ├── middleware/
│   │   └── auth.go             # JWT validation, JSON content-type
│   └── model/
│       └── model.go            # Domain types, request/response structs
├── migrations/                 # SQL migration files (up/down)
├── Dockerfile                  # Multi-stage build
├── go.mod
└── go.sum
```

## Design Decisions

- **Raw `database/sql` over an ORM.** At this scale, an ORM adds more abstraction than value. Parameterized queries throughout for SQL injection prevention.
- **`COALESCE` for partial updates.** PATCH endpoints use `COALESCE($1, column)` so only provided fields are updated. Tradeoff: fields cannot be explicitly set to null.
- **Added `creator_id` to tasks.** Not in the original spec, but necessary to enforce "only the task creator or project owner can delete/update a task."
- **Two queries for stats endpoint.** Could be combined into one, but separate queries are more readable at this scale.

## Getting Started

```bash
git clone <repo-url>
cd taskflow
cp .env.example .env
docker compose up --build
```

The API starts at `http://localhost:8080`. Migrations and seed data run automatically on startup.

### Test Credentials

Created by the seed script on first boot:

| Email | Password |
|-------|----------|
| `test@example.com` | `password123` |

## API Reference

A Postman collection is included at `postman/TaskFlow.postman_collection.json`. Import it, run the Login request first, and the token auto-populates for all other requests.

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/register` | Create a new account |
| POST | `/auth/login` | Authenticate and receive a JWT |

**Example:**

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'
```

### Projects

All endpoints require `Authorization: Bearer <token>`.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects` | List your projects (paginated) |
| POST | `/projects` | Create a new project |
| GET | `/projects/:id` | Get project details with tasks |
| PATCH | `/projects/:id` | Update project (owner only) |
| DELETE | `/projects/:id` | Delete project and its tasks (owner only) |
| GET | `/projects/:id/stats` | Task breakdown by status and assignee |

### Tasks

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects/:id/tasks` | List tasks (filterable, paginated) |
| POST | `/projects/:id/tasks` | Create a task in a project |
| PATCH | `/tasks/:id` | Update task (creator or project owner) |
| DELETE | `/tasks/:id` | Delete task (creator or project owner) |

**Query Parameters:**
- Filter: `?status=todo|in_progress|done`, `?assignee=<uuid>`
- Pagination: `?page=1&limit=20` (default limit: 20, max: 100)

### Error Responses

All errors return a consistent JSON structure:

```json
{"error": "validation failed", "fields": {"email": "is required"}}
```

| Status | Meaning |
|--------|---------|
| 400 | Validation error (with field details) |
| 401 | Missing or invalid authentication |
| 403 | Insufficient permissions |
| 404 | Resource not found |

## Authorization Model

- **Projects:** Only the project owner can update or delete a project.
- **Tasks:** Only the task creator or the project owner can update or delete a task.
- **Visibility:** Users see projects they own or are assigned tasks in.

## Testing

Integration tests cover auth flows (registration, login, validation, protected route access):

```bash
TEST_DATABASE_URL="postgres://user:pass@localhost:5432/testdb?sslmode=disable" \
  go test ./internal/handler/ -v
```

## Additional Features

- Pagination with total counts on all list endpoints
- `/projects/:id/stats` endpoint for task distribution by status and assignee
- Graceful server shutdown with signal handling
- Database connection retry on startup (up to 10 attempts)
- Optimized Docker layer caching for faster rebuilds

## Future Improvements

- Refresh token rotation (current: single JWT with 24h expiry)
- Rate limiting on authentication endpoints
- Project membership and invite system
- Connection pool tuning (`MaxOpenConns`, `MaxIdleConns`)
- `/health` endpoint for container orchestration
- Request ID propagation in all log lines
