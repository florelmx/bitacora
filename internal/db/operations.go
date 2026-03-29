package db

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	
	"github.com/florelmx/bitacora/internal/models"
)

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

// nowISO devuelve la fecha actual en formato ISO 8601 UTC.
// Es una función privada (minúscula) — solo se usa dentro de este paquete.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// NOTA: "2006-01-02T15:04:05Z" parece una fecha random pero es
// la forma de Go de definir formatos de fecha. Usa una fecha de referencia
// fija (Mon Jan 2 15:04:05 MST 2006) donde cada número es único.

// generateSessionID crea un ID legible para sesiones.
// Formato: session-20260324-143022-a7f3b2
func generateSessionID(project string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	// md5.Sum devuelve un array de 16 bytes. %x lo formatea como hexadecimal.
	// [:3] toma los primeros 3 bytes (6 caracteres hex).
	hash := fmt.Sprintf("%x", md5.Sum([]byte(ts+project+fmt.Sprint(time.Now().UnixNano()))))[:6]
	return fmt.Sprintf("session-%s-%s", ts, hash)
}

// toJSON convierte un slice de strings a JSON para guardar en SQLite.
// SQLite no tiene tipo array nativo, así que guardamos ["tag1", "tag2"] como texto.
func toJSON(items []string) string {
	if items == nil {
		return "[]"
	}
	data, _ := json.Marshal(items)
	return string(data)
}

// fromJSON parsea un string JSON a slice de strings.
func fromJSON(data string) []string {
	var items []string
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return []string{}
	}
	return items
}

// ─────────────────────────────────────────────
// Projects
// ─────────────────────────────────────────────

// CreateProject registra un proyecto nuevo.
// Si ya existe (mismo ID), no hace nada (INSERT OR IGNORE).
func (db *DB) CreateProject(p models.Project) error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO projects (id, name, path, git_remote, workspace, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Path, p.GitRemote, p.Workspace, nowISO(), nowISO(),
	)
	return err
}

// GetProject obtiene un proyecto por ID.
// Retorna el proyecto, un bool indicando si se encontró, y un error.
func (db *DB) GetProject(id string) (models.Project, bool, error) {
	var p models.Project
	var path, gitRemote, workspace sql.NullString
	var createdAt, updatedAt string

	err := db.conn.QueryRow(`
		SELECT id, name, path, git_remote, workspace, created_at, updated_at
		FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &path, &gitRemote, &workspace, &createdAt, &updatedAt)

	// NOTA sobre sql.NullString:
	// SQLite puede devolver NULL para columnas opcionales.
	// Go no puede meter NULL en un string normal, así que usa sql.NullString
	// que tiene dos campos: String (el valor) y Valid (si no es NULL).
	// Después lo convertimos a *string para nuestro struct.

	if err == sql.ErrNoRows {
		return models.Project{}, false, nil // No encontrado, sin error
	}
	if err != nil {
		return models.Project{}, false, err // Error real
	}

	// Convertir NullString a *string
	if path.Valid {
		p.Path = &path.String
	}
	if gitRemote.Valid {
		p.GitRemote = &gitRemote.String
	}
	if workspace.Valid {
		p.Workspace = &workspace.String
	}

	p.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
	p.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAt)

	return p, true, nil
}

// ─────────────────────────────────────────────
// Sessions
// ─────────────────────────────────────────────

// StartSession crea una nueva sesión y devuelve su ID.
func (db *DB) StartSession(projectID *string, objectives []string) (string, error) {
	project := ""
	if projectID != nil {
		project = *projectID
	}

	sessionID := generateSessionID(project)

	_, err := db.conn.Exec(`
		INSERT INTO sessions (id, project_id, objectives, status)
		VALUES (?, ?, ?, 'active')`,
		sessionID, projectID, toJSON(objectives),
	)
	if err != nil {
		return "", fmt.Errorf("error creando sesión: %w", err)
	}

	return sessionID, nil
}

// EndSession finaliza una sesión activa con resumen.
func (db *DB) EndSession(sessionID string, summary string, tasks []string, files []string) error {
	result, err := db.conn.Exec(`
		UPDATE sessions
		SET ended_at = ?, summary = ?, tasks_completed = ?,
		    files_touched = ?, status = 'completed'
		WHERE id = ? AND status = 'active'`,
		nowISO(), summary, toJSON(tasks), toJSON(files), sessionID,
	)
	if err != nil {
		return fmt.Errorf("error finalizando sesión: %w", err)
	}

	// RowsAffected() nos dice cuántas filas se actualizaron.
	// Si es 0, la sesión no existía o no estaba activa.
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("sesión '%s' no encontrada o no está activa", sessionID)
	}

	return nil
}

// GetActiveSession busca la sesión activa de un proyecto.
// Útil para saber si ya hay una sesión en curso.
func (db *DB) GetActiveSession(projectID *string) (*models.Session, error) {
	var s models.Session
	var projID sql.NullString
	var endedAt sql.NullString
	var summary sql.NullString
	var objectives, tasks, files string
	var startedAt string

	query := `SELECT id, project_id, started_at, ended_at, status,
	          objectives, summary, tasks_completed, files_touched, compaction_count
	          FROM sessions WHERE status = 'active'`

	var err error
	if projectID != nil {
		query += " AND project_id = ? ORDER BY started_at DESC LIMIT 1"
		err = db.conn.QueryRow(query, *projectID).Scan(
			&s.ID, &projID, &startedAt, &endedAt, &s.Status,
			&objectives, &summary, &tasks, &files, &s.CompactionCount,
		)
	} else {
		query += " AND project_id IS NULL ORDER BY started_at DESC LIMIT 1"
		err = db.conn.QueryRow(query).Scan(
			&s.ID, &projID, &startedAt, &endedAt, &s.Status,
			&objectives, &summary, &tasks, &files, &s.CompactionCount,
		)
	}

	if err == sql.ErrNoRows {
		return nil, nil // No hay sesión activa — no es error
	}
	if err != nil {
		return nil, err
	}

	// Mapear campos opcionales y JSON
	if projID.Valid {
		s.ProjectID = &projID.String
	}
	if summary.Valid {
		s.Summary = &summary.String
	}
	s.StartedAt, _ = time.Parse("2006-01-02T15:04:05Z", startedAt)
	if endedAt.Valid {
		t, _ := time.Parse("2006-01-02T15:04:05Z", endedAt.String)
		s.EndedAt = &t
	}
	s.Objectives = fromJSON(objectives)
	s.TasksCompleted = fromJSON(tasks)
	s.FilesTouched = fromJSON(files)

	return &s, nil
}

// ─────────────────────────────────────────────
// Observations
// ─────────────────────────────────────────────

// SaveObservation guarda una nueva observación y devuelve su ID.
// El trigger de FTS5 se ejecuta automáticamente al hacer INSERT.
func (db *DB) SaveObservation(input models.SaveObservationInput) (int64, error) {
	result, err := db.conn.Exec(`
		INSERT INTO observations
		(session_id, project_id, scope, category, title, content, tags, files)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		input.SessionID,
		input.ProjectID,
		string(input.Scope),
		string(input.Category),
		input.Title,
		input.Content,
		toJSON(input.Tags),
		toJSON(input.Files),
	)
	if err != nil {
		return 0, fmt.Errorf("error guardando observación: %w", err)
	}

	// LastInsertId() devuelve el ID auto-generado por SQLite.
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("error obteniendo ID: %w", err)
	}

	return id, nil
}

// GetObservation obtiene una observación por ID.
// Además incrementa access_count y actualiza last_accessed_at (reinforcement).
func (db *DB) GetObservation(id int) (models.Observation, bool, error) {
	// Primero actualizamos el acceso (reinforcement del relevance score)
	db.conn.Exec(`
		UPDATE observations
		SET access_count = access_count + 1,
		    last_accessed_at = ?,
		    relevance_score = MIN(relevance_score + 0.15, 2.0)
		WHERE id = ?`,
		nowISO(), id,
	)

	// Luego leemos la observación actualizada
	return db.scanObservation(
		db.conn.QueryRow(`
			SELECT id, session_id, project_id, scope, category, title, content,
			       tags, files, relevance_score, access_count, last_accessed_at,
			       superseded_by, is_active, created_at, updated_at
			FROM observations WHERE id = ?`, id),
	)
}

// scanObservation es un helper privado que convierte una fila SQL a un struct.
// Se reutiliza en GetObservation, Search, Browse, etc.
// Esto es el patrón DRY en Go — extraer la lógica repetitiva en helpers.
//
// NOTA: sql.Row y sql.Rows comparten la interfaz Scanner,
// así que este helper funciona con ambos.
type scanner interface {
	Scan(dest ...interface{}) error
}

func (db *DB) scanObservation(row scanner) (models.Observation, bool, error) {
	var o models.Observation
	var projectID, lastAccessed, supersededBy sql.NullString
	var tags, files string
	var createdAt, updatedAt string
	var scope, category string
	var isActive int

	err := row.Scan(
		&o.ID, &o.SessionID, &projectID, &scope, &category,
		&o.Title, &o.Content, &tags, &files,
		&o.RelevanceScore, &o.AccessCount, &lastAccessed,
		&supersededBy, &isActive, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return models.Observation{}, false, nil
	}
	if err != nil {
		return models.Observation{}, false, err
	}

	// Mapear campos
	o.Scope = models.Scope(scope)
	o.Category = models.Category(category)
	o.Tags = fromJSON(tags)
	o.Files = fromJSON(files)
	o.IsActive = isActive == 1
	o.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
	o.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAt)

	if projectID.Valid {
		o.ProjectID = &projectID.String
	}
	if lastAccessed.Valid {
		t, _ := time.Parse("2006-01-02T15:04:05Z", lastAccessed.String)
		o.LastAccessedAt = &t
	}
	if supersededBy.Valid {
		// Convertir string a int para el puntero
		var sid int
		fmt.Sscan(supersededBy.String, &sid)
		o.SupersededBy = &sid
	}

	return o, true, nil
}

// ─────────────────────────────────────────────
// Search (FTS5)
// ─────────────────────────────────────────────

// Search ejecuta una búsqueda full-text con FTS5.
// Combina el ranking BM25 (relevancia textual) con el effective_score
// (relevancia temporal con decay).
func (db *DB) Search(input models.SearchInput) ([]models.SearchResult, error) {
	// Sanitizar el query para FTS5
	safeQuery := sanitizeFTSQuery(input.Query)

	// Construir el query SQL dinámicamente según los filtros.
	// Los ? son placeholders — se reemplazan con los valores de args.
	// NUNCA concatenes valores directamente en el SQL.
	query := `
		SELECT o.id, o.session_id, o.project_id, o.scope, o.category,
		       o.title, o.content, o.tags, o.files,
		       o.relevance_score, o.access_count, o.last_accessed_at,
		       o.superseded_by, o.is_active, o.created_at, o.updated_at,
		       rank * -1 * o.relevance_score * POWER(0.99,
		           JULIANDAY('now') - JULIANDAY(COALESCE(o.last_accessed_at, o.created_at))
		       ) AS effective_score
		FROM observations_fts fts
		JOIN observations o ON o.id = fts.rowid
		WHERE observations_fts MATCH ?
		  AND o.is_active = 1`

	// args es un slice de interface{} — puede contener cualquier tipo.
	args := []interface{}{safeQuery}

	if input.Category != nil {
		query += " AND o.category = ?"
		args = append(args, string(*input.Category))
	}

	if input.Project != nil {
		query += " AND o.project_id = ?"
		args = append(args, *input.Project)
	}

	if input.Scope != nil {
		query += " AND o.scope = ?"
		args = append(args, string(*input.Scope))
	}

	query += " ORDER BY effective_score DESC LIMIT ?"
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit)

	// Ejecutar query. db.conn.Query devuelve un iterador de filas.
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("error en búsqueda FTS5: %w", err)
	}
	// defer rows.Close() se asegura de liberar el cursor.
	// Sin esto, la conexión queda bloqueada.
	defer rows.Close()

	var results []models.SearchResult

	// rows.Next() avanza al siguiente resultado. Retorna false cuando no hay más.
	for rows.Next() {
		var o models.Observation
		var projectID, lastAccessed, supersededBy sql.NullString
		var tags, files string
		var createdAt, updatedAt string
		var scope, category string
		var isActive int
		var effectiveScore float64

		var obsID string

		err := rows.Scan(
			&obsID, &o.SessionID, &projectID, &scope, &category,
			&o.Title, &o.Content, &tags, &files,
			&o.RelevanceScore, &o.AccessCount, &lastAccessed,
			&supersededBy, &isActive, &createdAt, &updatedAt,
			&effectiveScore,
		)
		if err != nil {
			return nil, fmt.Errorf("error escaneando resultado: %w", err)
		}

		o.ID, _ = strconv.Atoi(obsID)

		// Mapear campos (mismo patrón que scanObservation)
		o.Scope = models.Scope(scope)
		o.Category = models.Category(category)
		o.Tags = fromJSON(tags)
		o.Files = fromJSON(files)
		o.IsActive = isActive == 1
		o.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		o.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAt)
		if projectID.Valid {
			o.ProjectID = &projectID.String
		}
		if lastAccessed.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", lastAccessed.String)
			o.LastAccessedAt = &t
		}
		if supersededBy.Valid {
			var sid int
			fmt.Sscan(supersededBy.String, &sid)
			o.SupersededBy = &sid
		}

		results = append(results, models.SearchResult{
			Observation:    o,
			EffectiveScore: effectiveScore,
		})
	}

	// rows.Err() captura errores que ocurrieron durante la iteración.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// sanitizeFTSQuery limpia un query de usuario para FTS5.
func sanitizeFTSQuery(query string) string {
	// strings.NewReplacer maneja UTF-8 correctamente.
	// Reemplaza caracteres especiales de FTS5 por espacios.
	replacer := strings.NewReplacer(
		"\"", " ", "'", " ", "(", " ", ")", " ",
		"{", " ", "}", " ", "[", " ", "]", " ",
		"^", " ", "~", " ", ":", " ", ";", " ",
	)
	cleaned := replacer.Replace(query)

	// strings.Fields divide por cualquier whitespace y filtra vacíos.
	// A diferencia de strings.Split, maneja múltiples espacios seguidos.
	// Y lo más importante: maneja UTF-8 correctamente (runes, no bytes).
	tokens := strings.Fields(cleaned)

	if len(tokens) == 0 {
		return query
	}
	if len(tokens) == 1 {
		return tokens[0]
	}

	// Unir con espacio (FTS5 usa AND implícito)
	return strings.Join(tokens, " ")
}

// ─────────────────────────────────────────────
// Relations (grafo de conocimiento)
// ─────────────────────────────────────────────

// CreateRelation conecta dos observaciones.
// El UNIQUE constraint evita duplicados del mismo tipo entre el mismo par.
func (db *DB) CreateRelation(sourceID, targetID int, relType models.RelationType, description *string) (int64, error) {
	result, err := db.conn.Exec(`
		INSERT INTO relations (source_id, target_id, relation_type, description)
		VALUES (?, ?, ?, ?)`,
		sourceID, targetID, string(relType), description,
	)
	if err != nil {
		return 0, fmt.Errorf("error creando relación: %w", err)
	}

	// Si la relación es "supersedes", marcar la observación vieja como inactiva
	if relType == models.RelSupersedes {
		db.conn.Exec(`
			UPDATE observations
			SET superseded_by = ?, is_active = 0, updated_at = ?
			WHERE id = ?`,
			sourceID, nowISO(), targetID,
		)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// GetRelations obtiene todas las relaciones de una observación.
// Busca en ambas direcciones: donde la observación es source Y donde es target.
func (db *DB) GetRelations(observationID int) ([]models.Relation, error) {
	rows, err := db.conn.Query(`
		SELECT id, source_id, target_id, relation_type, description, created_at
		FROM relations
		WHERE source_id = ? OR target_id = ?
		ORDER BY created_at DESC`,
		observationID, observationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []models.Relation
	for rows.Next() {
		var r models.Relation
		var desc sql.NullString
		var relType, createdAt string

		err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &relType, &desc, &createdAt)
		if err != nil {
			return nil, err
		}

		r.RelationType = models.RelationType(relType)
		r.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if desc.Valid {
			r.Description = &desc.String
		}

		relations = append(relations, r)
	}

	return relations, rows.Err()
}

// ─────────────────────────────────────────────
// User Requests
// ─────────────────────────────────────────────

// SaveRequest guarda una petición del usuario.
func (db *DB) SaveRequest(sessionID string, projectID *string, request string, priority models.RequestPriority) (int64, error) {
	result, err := db.conn.Exec(`
		INSERT INTO user_requests (session_id, project_id, request, priority)
		VALUES (?, ?, ?, ?)`,
		sessionID, projectID, request, string(priority),
	)
	if err != nil {
		return 0, fmt.Errorf("error guardando petición: %w", err)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// UpdateRequestStatus actualiza el estado de una petición.
func (db *DB) UpdateRequestStatus(id int, status models.RequestStatus, resolution *string) error {
	var completedAt *string
	if status == models.RequestCompleted {
		now := nowISO()
		completedAt = &now
	}

	result, err := db.conn.Exec(`
		UPDATE user_requests
		SET status = ?, resolution = ?, completed_at = ?
		WHERE id = ?`,
		string(status), resolution, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("error actualizando petición: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("petición #%d no encontrada", id)
	}
	return nil
}

// GetPendingRequests obtiene peticiones pendientes o en progreso.
func (db *DB) GetPendingRequests(projectID *string, limit int) ([]models.UserRequest, error) {
	query := `
		SELECT id, session_id, project_id, request, priority, status,
		       resolution, created_at, completed_at
		FROM user_requests
		WHERE status IN ('pending', 'in_progress')`

	args := []interface{}{}

	if projectID != nil {
		query += " AND project_id = ?"
		args = append(args, *projectID)
	}

	query += " ORDER BY CASE priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END, created_at DESC LIMIT ?"
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []models.UserRequest
	for rows.Next() {
		var r models.UserRequest
		var projID, resolution, completedAt sql.NullString
		var priority, status, createdAt string

		err := rows.Scan(
			&r.ID, &r.SessionID, &projID, &r.Request,
			&priority, &status, &resolution, &createdAt, &completedAt,
		)
		if err != nil {
			return nil, err
		}

		r.Priority = models.RequestPriority(priority)
		r.Status = models.RequestStatus(status)
		r.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if projID.Valid {
			r.ProjectID = &projID.String
		}
		if resolution.Valid {
			r.Resolution = &resolution.String
		}
		if completedAt.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", completedAt.String)
			r.CompletedAt = &t
		}

		requests = append(requests, r)
	}

	return requests, rows.Err()
}

// ─────────────────────────────────────────────
// Context (lo que devuelve bit_context)
// ─────────────────────────────────────────────

// GetContext construye el contexto completo para una sesión.
// Combina: sesiones recientes + observaciones por categoría + peticiones pendientes.
// Los tres scopes (global + project + workspace) se combinan automáticamente.
func (db *DB) GetContext(input models.ContextInput) (models.ContextResponse, error) {
	ctx := models.ContextResponse{}
	perCategory := 10

	// Sesiones recientes del proyecto
	ctx.RecentSessions = db.getRecentSessions(input.Project, 5)

	// Observaciones por categoría, combinando scopes.
	// Para cada categoría buscamos: las del proyecto + las globales + las del workspace.
	ctx.Decisions = db.getObservationsByCategory("decision", input.Project, input.Workspace, perCategory)
	ctx.Bugs = db.getObservationsByCategory("bug", input.Project, input.Workspace, perCategory)
	ctx.Patterns = db.getObservationsByCategory("pattern", input.Project, input.Workspace, perCategory)
	ctx.Notes = db.getObservationsByCategory("note", input.Project, input.Workspace, perCategory)

	// Peticiones pendientes
	if input.IncludeRequests {
		reqs, _ := db.GetPendingRequests(input.Project, perCategory)
		ctx.PendingRequests = reqs
	}

	ctx.TotalItems = len(ctx.RecentSessions) + len(ctx.Decisions) +
		len(ctx.Bugs) + len(ctx.Patterns) + len(ctx.Notes) + len(ctx.PendingRequests)

	return ctx, nil
}

// getRecentSessions obtiene las últimas sesiones de un proyecto.
func (db *DB) getRecentSessions(projectID *string, limit int) []models.Session {
	query := "SELECT id, project_id, started_at, ended_at, status, objectives, summary, tasks_completed, files_touched, compaction_count FROM sessions"
	args := []interface{}{}

	if projectID != nil {
		query += " WHERE project_id = ?"
		args = append(args, *projectID)
	}
	query += " ORDER BY started_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		var projID, endedAt, summary sql.NullString
		var objectives, tasks, files, startedAt string

		err := rows.Scan(
			&s.ID, &projID, &startedAt, &endedAt, &s.Status,
			&objectives, &summary, &tasks, &files, &s.CompactionCount,
		)
		if err != nil {
			continue
		}

		s.StartedAt, _ = time.Parse("2006-01-02T15:04:05Z", startedAt)
		if projID.Valid {
			s.ProjectID = &projID.String
		}
		if endedAt.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", endedAt.String)
			s.EndedAt = &t
		}
		if summary.Valid {
			s.Summary = &summary.String
		}
		s.Objectives = fromJSON(objectives)
		s.TasksCompleted = fromJSON(tasks)
		s.FilesTouched = fromJSON(files)

		sessions = append(sessions, s)
	}

	return sessions
}

// getObservationsByCategory obtiene observaciones de una categoría
// combinando los tres scopes: project + workspace + global.
// Usa la view observations_ranked que calcula el effective_score con decay.
func (db *DB) getObservationsByCategory(category string, projectID *string, workspace *string, limit int) []models.Observation {
	query := `
		SELECT id, session_id, project_id, scope, category, title, content,
		       tags, files, relevance_score, access_count, last_accessed_at,
		       superseded_by, is_active, created_at, updated_at
		FROM observations_ranked
		WHERE category = ?
		AND (`

	args := []interface{}{category}

	// Combinar scopes: global siempre + project si hay + workspace si hay
	conditions := []string{"scope = 'global'"}

	if projectID != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *projectID)
	}
	if workspace != nil {
		conditions = append(conditions, "(scope = 'workspace' AND project_id IN (SELECT id FROM projects WHERE workspace = ?))")
		args = append(args, *workspace)
	}

	// Unir condiciones con OR
	for i, cond := range conditions {
		if i > 0 {
			query += " OR "
		}
		query += cond
	}

	query += ") ORDER BY effective_score DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var observations []models.Observation
	for rows.Next() {
		var o models.Observation
		var projectID, lastAccessed, supersededBy sql.NullString
		var tags, files, createdAt, updatedAt string
		var scope, cat string
		var isActive int

		err := rows.Scan(
			&o.ID, &o.SessionID, &projectID, &scope, &cat,
			&o.Title, &o.Content, &tags, &files,
			&o.RelevanceScore, &o.AccessCount, &lastAccessed,
			&supersededBy, &isActive, &createdAt, &updatedAt,
		)
		if err != nil {
			continue
		}

		o.Scope = models.Scope(scope)
		o.Category = models.Category(cat)
		o.Tags = fromJSON(tags)
		o.Files = fromJSON(files)
		o.IsActive = isActive == 1
		o.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		o.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", updatedAt)
		if projectID.Valid {
			o.ProjectID = &projectID.String
		}
		if lastAccessed.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", lastAccessed.String)
			o.LastAccessedAt = &t
		}
		if supersededBy.Valid {
			var sid int
			fmt.Sscan(supersededBy.String, &sid)
			o.SupersededBy = &sid
		}

		observations = append(observations, o)
	}

	return observations
}

// ─────────────────────────────────────────────
// Compaction Snapshots
// ─────────────────────────────────────────────

// SaveSnapshot guarda un snapshot de la transcripción.
func (db *DB) SaveSnapshot(sessionID string, snapshotType string, summary string, rawLength *int) (int64, error) {
	result, err := db.conn.Exec(`
		INSERT INTO compaction_snapshots (session_id, snapshot_type, summary, raw_length)
		VALUES (?, ?, ?, ?)`,
		sessionID, snapshotType, summary, rawLength,
	)
	if err != nil {
		return 0, fmt.Errorf("error guardando snapshot: %w", err)
	}

	id, _ := result.LastInsertId()
	return id, nil
}

// ─────────────────────────────────────────────
// Stats
// ─────────────────────────────────────────────

// Stats devuelve estadísticas generales del sistema.
type Stats struct {
	TotalObservations int            `json:"total_observations"`
	ActiveObservations int           `json:"active_observations"`
	TotalSessions     int            `json:"total_sessions"`
	ActiveSessions    int            `json:"active_sessions"`
	TotalRequests     int            `json:"total_requests"`
	PendingRequests   int            `json:"pending_requests"`
	TotalRelations    int            `json:"total_relations"`
	TotalProjects     int            `json:"total_projects"`
	ByCategory        map[string]int `json:"by_category"`
	ByScope           map[string]int `json:"by_scope"`
}

func (db *DB) GetStats() (Stats, error) {
	s := Stats{
		ByCategory: make(map[string]int),
		ByScope:    make(map[string]int),
	}

	// Contadores simples. Cada QueryRow + Scan lee un solo valor.
	db.conn.QueryRow("SELECT COUNT(*) FROM observations").Scan(&s.TotalObservations)
	db.conn.QueryRow("SELECT COUNT(*) FROM observations WHERE is_active = 1").Scan(&s.ActiveObservations)
	db.conn.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&s.TotalSessions)
	db.conn.QueryRow("SELECT COUNT(*) FROM sessions WHERE status = 'active'").Scan(&s.ActiveSessions)
	db.conn.QueryRow("SELECT COUNT(*) FROM user_requests").Scan(&s.TotalRequests)
	db.conn.QueryRow("SELECT COUNT(*) FROM user_requests WHERE status IN ('pending', 'in_progress')").Scan(&s.PendingRequests)
	db.conn.QueryRow("SELECT COUNT(*) FROM relations").Scan(&s.TotalRelations)
	db.conn.QueryRow("SELECT COUNT(*) FROM projects").Scan(&s.TotalProjects)

	// Conteo por categoría
	rows, err := db.conn.Query("SELECT category, COUNT(*) FROM observations WHERE is_active = 1 GROUP BY category")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat string
			var count int
			rows.Scan(&cat, &count)
			s.ByCategory[cat] = count
		}
	}

	// Conteo por scope
	rows2, err := db.conn.Query("SELECT scope, COUNT(*) FROM observations WHERE is_active = 1 GROUP BY scope")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var scope string
			var count int
			rows2.Scan(&scope, &count)
			s.ByScope[scope] = count
		}
	}

	return s, nil
}

// GetRecentSessionsFiltered obtiene sesiones con filtro opcional de estado.
// Es un método público (mayúscula) pero con nombre en minúscula
// porque lo usaremos internamente desde el paquete mcp.
func (db *DB) GetRecentSessionsFiltered(projectID *string, status string, limit int) []models.Session {
	query := "SELECT id, project_id, started_at, ended_at, status, objectives, summary, tasks_completed, files_touched, compaction_count FROM sessions WHERE 1=1"
	args := []interface{}{}

	if projectID != nil {
		query += " AND project_id = ?"
		args = append(args, *projectID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY started_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		var projID, endedAt, summary sql.NullString
		var objectives, tasks, files, startedAt string

		err := rows.Scan(
			&s.ID, &projID, &startedAt, &endedAt, &s.Status,
			&objectives, &summary, &tasks, &files, &s.CompactionCount,
		)
		if err != nil {
			continue
		}

		s.StartedAt, _ = time.Parse("2006-01-02T15:04:05Z", startedAt)
		if projID.Valid {
			s.ProjectID = &projID.String
		}
		if endedAt.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", endedAt.String)
			s.EndedAt = &t
		}
		if summary.Valid {
			s.Summary = &summary.String
		}
		s.Objectives = fromJSON(objectives)
		s.TasksCompleted = fromJSON(tasks)
		s.FilesTouched = fromJSON(files)

		sessions = append(sessions, s)
	}

	return sessions
}