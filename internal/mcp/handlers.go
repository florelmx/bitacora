package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/florelmx/bitacora/internal/db"
	"github.com/florelmx/bitacora/internal/models"
)

// handlers agrupa todos los tool handlers.
// Cada handler necesita acceso a la DB, así que la referencia vive aquí.
type handlers struct {
	db *db.DB
}

// ── Helpers para extraer parámetros ──
// Los argumentos del agente vienen como map[string]interface{}.
// Estos helpers extraen el valor con el tipo correcto.
// Usan type assertions de Go para obtener el tipo correcto.

// getArgs extrae los argumentos como map.
// request.Params.Arguments viene como `any` en mcp-go,
// así que necesitamos un type assertion para convertirlo.
func getArgs(request mcp.CallToolRequest) map[string]interface{} {
	if args, ok := request.Params.Arguments.(map[string]interface{}); ok {
		return args
	}
	return map[string]interface{}{}
}

func getString(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStringPtr(args map[string]interface{}, key string) *string {
	s := getString(args, key)
	if s == "" {
		return nil
	}
	return &s
}

func getInt(args map[string]interface{}, key string) int {
	if v, ok := args[key]; ok {
		// JSON numbers llegan como float64 en Go (igual que en JS)
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

func getBool(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// toResult convierte cualquier valor a una respuesta MCP exitosa.
// Serializa a JSON y lo envuelve en el formato que MCP espera.
func toResult(data interface{}) *mcp.CallToolResult {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errResult(fmt.Sprintf("Error serializando respuesta: %v", err))
	}
	return mcp.NewToolResultText(string(jsonBytes))
}

// errResult devuelve un error en formato MCP.
func errResult(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultError(msg)
}

// ─────────────────────────────────────────────
// bit_start_session
// ─────────────────────────────────────────────

// Cada handler tiene la misma firma:
//   func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
//
// ctx es el contexto de Go (para timeouts y cancelación, no confundir con "contexto de Bitácora").
// request contiene los argumentos que el agente envió.
// Retorna el resultado o un error.

func (h *handlers) startSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	projectID := getString(args, "project_id")
	if projectID == "" {
		return errResult("project_id es requerido"), nil
	}

	// Crear o actualizar el proyecto
	projectName := getString(args, "project_name")
	if projectName == "" {
		projectName = projectID
	}

	h.db.CreateProject(models.Project{
		ID:        projectID,
		Name:      projectName,
		Path:      getStringPtr(args, "project_path"),
		GitRemote: getStringPtr(args, "git_remote"),
		Workspace: getStringPtr(args, "workspace"),
	})

	// Iniciar sesión
	objectives := []string{}
	sessionID, err := h.db.StartSession(&projectID, objectives)
	if err != nil {
		return errResult(fmt.Sprintf("Error iniciando sesión: %v", err)), nil
	}

	// Cargar contexto del proyecto automáticamente
	ctxResponse, _ := h.db.GetContext(models.ContextInput{
		Project:         &projectID,
		Workspace:       getStringPtr(args, "workspace"),
		MaxTokens:       4000,
		IncludeRequests: true,
	})

	return toResult(map[string]interface{}{
		"session_id":     sessionID,
		"project_id":     projectID,
		"recent_context": ctxResponse,
		"message":        fmt.Sprintf("Sesión iniciada. %d items de contexto previo cargados.", ctxResponse.TotalItems),
	}), nil
}

// ─────────────────────────────────────────────
// bit_end_session
// ─────────────────────────────────────────────

func (h *handlers) endSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	sessionID := getString(args, "session_id")
	if sessionID == "" {
		return errResult("session_id es requerido"), nil
	}

	summary := getString(args, "summary")
	if summary == "" {
		summary = "Sesión finalizada sin resumen detallado."
	}

	err := h.db.EndSession(sessionID, summary, nil, nil)
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}

	// Guardar snapshot de fin de sesión
	h.db.SaveSnapshot(sessionID, "session_end", summary, nil)

	return toResult(map[string]interface{}{
		"session_id": sessionID,
		"status":     "completed",
		"message":    "Sesión finalizada y resumen guardado.",
	}), nil
}

// ─────────────────────────────────────────────
// bit_save
// ─────────────────────────────────────────────

func (h *handlers) saveObservation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	sessionID := getString(args, "session_id")
	category := getString(args, "category")
	title := getString(args, "title")
	content := getString(args, "content")

	if sessionID == "" || category == "" || title == "" || content == "" {
		return errResult("session_id, category, title y content son requeridos"), nil
	}

	scope := getString(args, "scope")
	if scope == "" {
		scope = "project"
	}

	id, err := h.db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: getStringPtr(args, "project_id"),
		Scope:     models.Scope(scope),
		Category:  models.Category(category),
		Title:     title,
		Content:   content,
	})
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}

	return toResult(map[string]interface{}{
		"id":       id,
		"category": category,
		"scope":    scope,
		"title":    title,
		"message":  fmt.Sprintf("%s guardada en memoria (id: %d).", category, id),
	}), nil
}

// ─────────────────────────────────────────────
// bit_search
// ─────────────────────────────────────────────

func (h *handlers) search(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	query := getString(args, "query")
	if query == "" {
		return errResult("query es requerido"), nil
	}

	limit := getInt(args, "limit")
	if limit <= 0 {
		limit = 20
	}

	input := models.SearchInput{
		Query:   query,
		Limit:   limit,
		Project: getStringPtr(args, "project"),
	}

	// Filtros opcionales con conversión a puntero tipado
	if cat := getString(args, "category"); cat != "" {
		c := models.Category(cat)
		input.Category = &c
	}
	if scope := getString(args, "scope"); scope != "" {
		s := models.Scope(scope)
		input.Scope = &s
	}

	results, err := h.db.Search(input)
	if err != nil {
		return errResult(fmt.Sprintf("Error en búsqueda: %v", err)), nil
	}

	return toResult(map[string]interface{}{
		"query":       query,
		"total_found": len(results),
		"results":     results,
	}), nil
}

// ─────────────────────────────────────────────
// bit_context
// ─────────────────────────────────────────────

func (h *handlers) getContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	input := models.ContextInput{
		Project:         getStringPtr(args, "project"),
		Workspace:       getStringPtr(args, "workspace"),
		MaxTokens:       4000,
		IncludeRequests: getBool(args, "include_requests", true),
	}

	ctxResponse, err := h.db.GetContext(input)
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}

	return toResult(ctxResponse), nil
}

// ─────────────────────────────────────────────
// bit_get
// ─────────────────────────────────────────────

func (h *handlers) getObservation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)
	id := getInt(args, "id")
	if id == 0 {
		return errResult("id es requerido"), nil
	}

	obs, found, err := h.db.GetObservation(id)
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}
	if !found {
		return errResult(fmt.Sprintf("Observación #%d no encontrada", id)), nil
	}

	// También traer relaciones
	relations, _ := h.db.GetRelations(id)

	return toResult(map[string]interface{}{
		"observation": obs,
		"relations":   relations,
	}), nil
}

// ─────────────────────────────────────────────
// bit_save_request
// ─────────────────────────────────────────────

func (h *handlers) saveRequest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	sessionID := getString(args, "session_id")
	reqText := getString(args, "request")
	if sessionID == "" || reqText == "" {
		return errResult("session_id y request son requeridos"), nil
	}

	priority := getString(args, "priority")
	if priority == "" {
		priority = "normal"
	}

	id, err := h.db.SaveRequest(sessionID, getStringPtr(args, "project_id"), reqText, models.RequestPriority(priority))
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}

	return toResult(map[string]interface{}{
		"id":       id,
		"status":   "pending",
		"priority": priority,
		"message":  "Petición guardada. Aparecerá en el contexto de próximas sesiones.",
	}), nil
}

// ─────────────────────────────────────────────
// bit_update_request
// ─────────────────────────────────────────────

func (h *handlers) updateRequest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	id := getInt(args, "id")
	status := getString(args, "status")
	if id == 0 || status == "" {
		return errResult("id y status son requeridos"), nil
	}

	err := h.db.UpdateRequestStatus(id, models.RequestStatus(status), getStringPtr(args, "resolution"))
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}

	return toResult(map[string]interface{}{
		"id":      id,
		"status":  status,
		"message": fmt.Sprintf("Petición #%d actualizada a '%s'.", id, status),
	}), nil
}

// ─────────────────────────────────────────────
// bit_relate
// ─────────────────────────────────────────────

func (h *handlers) createRelation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	sourceID := getInt(args, "source_id")
	targetID := getInt(args, "target_id")
	relType := getString(args, "relation_type")

	if sourceID == 0 || targetID == 0 || relType == "" {
		return errResult("source_id, target_id y relation_type son requeridos"), nil
	}

	id, err := h.db.CreateRelation(sourceID, targetID, models.RelationType(relType), getStringPtr(args, "description"))
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}

	return toResult(map[string]interface{}{
		"id":            id,
		"source_id":     sourceID,
		"target_id":     targetID,
		"relation_type": relType,
		"message":       fmt.Sprintf("Relación creada: #%d -[%s]-> #%d", sourceID, relType, targetID),
	}), nil
}

// ─────────────────────────────────────────────
// bit_stats
// ─────────────────────────────────────────────

func (h *handlers) getStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stats, err := h.db.GetStats()
	if err != nil {
		return errResult(fmt.Sprintf("Error: %v", err)), nil
	}
	return toResult(stats), nil
}

// ─────────────────────────────────────────────
// bit_list_sessions
// ─────────────────────────────────────────────

func (h *handlers) listSessions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	limit := getInt(args, "limit")
	if limit <= 0 {
		limit = 10
	}

	project := getStringPtr(args, "project")

	sessions := h.db.GetRecentSessionsFiltered(project, getString(args, "status"), limit)

	return toResult(map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}), nil
}