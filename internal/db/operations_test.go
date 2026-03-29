package db

import (
	"path/filepath"
	"testing"

	"github.com/florelmx/bitacora/internal/models"
)

// setupTestDB crea una base de datos temporal para cada test.
// Cada test tiene su propia DB limpia — no comparten estado.
func setupTestDB(t *testing.T) *DB {
	// t.TempDir() crea un directorio temporal que Go limpia automáticamente al terminar el test
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenAt(dbPath)
	if err != nil {
		t.Fatalf("error abriendo DB de test: %v", err)
	}

	// t.Cleanup registra una función que se ejecuta al terminar el test (como defer pero para tests)
	t.Cleanup(func() { db.Close() })

	return db
}

// ─────────────────────────────────────────────
// Projects
// ─────────────────────────────────────────────

func TestCreateProject(t *testing.T) {
	db := setupTestDB(t)

	path := "/home/user/mi-app"
	remote := "github.com/user/mi-app"

	err := db.CreateProject(models.Project{
		ID:        "mi-app",
		Name:      "Mi App",
		Path:      &path,
		GitRemote: &remote,
	})
	if err != nil {
		t.Fatalf("error creando proyecto: %v", err)
	}

	// Verificar que se creó
	p, found, err := db.GetProject("mi-app")
	if err != nil {
		t.Fatalf("error obteniendo proyecto: %v", err)
	}
	if !found {
		t.Fatal("proyecto no encontrado")
	}
	if p.Name != "Mi App" {
		t.Errorf("nombre esperado 'Mi App', got '%s'", p.Name)
	}
	if p.Path == nil || *p.Path != path {
		t.Error("path no coincide")
	}
}

func TestCreateProjectIdempotent(t *testing.T) {
	db := setupTestDB(t)

	// Crear el mismo proyecto dos veces no debe dar error (INSERT OR IGNORE)
	db.CreateProject(models.Project{ID: "app", Name: "App 1"})
	err := db.CreateProject(models.Project{ID: "app", Name: "App 2"})
	if err != nil {
		t.Fatalf("segundo CreateProject debería ser idempotente: %v", err)
	}

	// El nombre debe ser el original (IGNORE no actualiza)
	p, _, _ := db.GetProject("app")
	if p.Name != "App 1" {
		t.Errorf("esperado 'App 1', got '%s'", p.Name)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	db := setupTestDB(t)

	_, found, err := db.GetProject("no-existe")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if found {
		t.Error("no debería encontrar un proyecto que no existe")
	}
}

// ─────────────────────────────────────────────
// Sessions
// ─────────────────────────────────────────────

func TestStartAndEndSession(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})

	// Iniciar sesión
	sessionID, err := db.StartSession(&projectID, []string{"objetivo 1", "objetivo 2"})
	if err != nil {
		t.Fatalf("error iniciando sesión: %v", err)
	}
	if sessionID == "" {
		t.Fatal("session ID vacío")
	}

	// Verificar sesión activa
	active, err := db.GetActiveSession(&projectID)
	if err != nil {
		t.Fatalf("error buscando sesión activa: %v", err)
	}
	if active == nil {
		t.Fatal("no se encontró sesión activa")
	}
	if active.ID != sessionID {
		t.Errorf("sesión activa esperada '%s', got '%s'", sessionID, active.ID)
	}

	// Finalizar sesión
	err = db.EndSession(sessionID, "Resumen de test", []string{"tarea 1"}, []string{"file.go"})
	if err != nil {
		t.Fatalf("error finalizando sesión: %v", err)
	}

	// Ya no debe haber sesión activa
	active, _ = db.GetActiveSession(&projectID)
	if active != nil {
		t.Error("no debería haber sesión activa después de EndSession")
	}
}

func TestEndSessionNotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.EndSession("sesion-inexistente", "resumen", nil, nil)
	if err == nil {
		t.Error("debería dar error al finalizar sesión inexistente")
	}
}

// ─────────────────────────────────────────────
// Observations
// ─────────────────────────────────────────────

func TestSaveAndGetObservation(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	// Guardar observación
	id, err := db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryDecision,
		Title:     "Usar JWT para auth",
		Content:   "What: JWT sobre sessions. Why: API stateless.",
		Tags:      []string{"auth", "jwt"},
		Files:     []string{"src/auth.go"},
	})
	if err != nil {
		t.Fatalf("error guardando observación: %v", err)
	}
	if id == 0 {
		t.Fatal("ID debe ser > 0")
	}

	// Leer observación (esto también incrementa access_count)
	obs, found, err := db.GetObservation(int(id))
	if err != nil {
		t.Fatalf("error obteniendo observación: %v", err)
	}
	if !found {
		t.Fatal("observación no encontrada")
	}
	if obs.Title != "Usar JWT para auth" {
		t.Errorf("título esperado 'Usar JWT para auth', got '%s'", obs.Title)
	}
	if obs.AccessCount != 1 {
		t.Errorf("access_count esperado 1, got %d", obs.AccessCount)
	}
	if obs.RelevanceScore <= 1.0 {
		t.Error("relevance_score debe haber aumentado por reinforcement")
	}
}

func TestObservationScopes(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	// Guardar una observación global y una de proyecto
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		Scope:     models.ScopeGlobal,
		Category:  models.CategoryPreference,
		Title:     "Usar conventional commits",
		Content:   "Aplica a todos los proyectos",
	})
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryDecision,
		Title:     "Usar PostgreSQL",
		Content:   "Solo para este proyecto",
	})

	// El contexto del proyecto debe incluir ambas (global + project)
	ctx, err := db.GetContext(models.ContextInput{
		Project:         &projectID,
		MaxTokens:       4000,
		IncludeRequests: false,
	})
	if err != nil {
		t.Fatalf("error obteniendo contexto: %v", err)
	}

	totalObs := len(ctx.Decisions) + len(ctx.Patterns) + len(ctx.Bugs) + len(ctx.Notes) + len(ctx.Preferences)
	if totalObs < 2 {
		t.Errorf("contexto debe incluir observaciones global + project, got %d", totalObs)
	}
}

// ─────────────────────────────────────────────
// FTS5 Search
// ─────────────────────────────────────────────

func TestSearchFTS5(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	// Guardar varias observaciones
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryDecision,
		Title:     "Usar JWT para autenticación",
		Content:   "Elegimos JWT sobre sessions por escalabilidad",
	})
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryBug,
		Title:     "Race condition en refresh de tokens",
		Content:   "Dos requests simultáneos causan doble refresh",
	})
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryPattern,
		Title:     "Repository pattern para data access",
		Content:   "Cada entidad tiene su repositorio CRUD",
	})

	// Buscar por texto
	results, err := db.Search(models.SearchInput{Query: "JWT autenticación", Limit: 10})
	if err != nil {
		t.Fatalf("error en búsqueda: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("búsqueda 'JWT autenticación' debe encontrar resultados")
	}
	if results[0].Observation.Title != "Usar JWT para autenticación" {
		t.Errorf("primer resultado esperado 'Usar JWT...', got '%s'", results[0].Observation.Title)
	}

	// Buscar con filtro de categoría
	bugCat := models.CategoryBug
	results2, _ := db.Search(models.SearchInput{Query: "tokens", Category: &bugCat, Limit: 10})
	if len(results2) == 0 {
		t.Fatal("búsqueda 'tokens' con category=bug debe encontrar resultados")
	}
	if results2[0].Observation.Category != models.CategoryBug {
		t.Error("resultado filtrado debe ser categoría bug")
	}

	// Buscar algo que no existe
	results3, _ := db.Search(models.SearchInput{Query: "kubernetes docker", Limit: 10})
	if len(results3) != 0 {
		t.Errorf("búsqueda de algo inexistente debe retornar 0 resultados, got %d", len(results3))
	}
}

func TestSearchUnicodeAccents(t *testing.T) {
	db := setupTestDB(t)

	db.CreateProject(models.Project{ID: "p", Name: "P"})
	pid := "p"

	realSessionID, _ := db.StartSession(&pid, nil)

	db.SaveObservation(models.SaveObservationInput{
		SessionID: realSessionID,
		Scope:     models.ScopeGlobal,
		Category:  models.CategoryNote,
		Title:     "Configuración de autenticación",
		Content:   "Método de conexión único",
	})

	// Buscar SIN acento debe encontrar resultado CON acento
	results, _ := db.Search(models.SearchInput{Query: "autenticacion", Limit: 10})
	if len(results) == 0 {
		t.Fatal("FTS5 con remove_diacritics debe encontrar 'autenticación' buscando 'autenticacion'")
	}
}

// ─────────────────────────────────────────────
// Relations
// ─────────────────────────────────────────────

func TestCreateRelation(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	id1, _ := db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryDecision,
		Title: "Decisión A", Content: "Contenido A",
	})
	id2, _ := db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryBug,
		Title: "Bug B", Content: "Contenido B",
	})

	// Crear relación
	desc := "La decisión se tomó por este bug"
	relID, err := db.CreateRelation(int(id1), int(id2), models.RelCausedBy, &desc)
	if err != nil {
		t.Fatalf("error creando relación: %v", err)
	}
	if relID == 0 {
		t.Fatal("relación ID debe ser > 0")
	}

	// Obtener relaciones
	rels, err := db.GetRelations(int(id1))
	if err != nil {
		t.Fatalf("error obteniendo relaciones: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("esperada 1 relación, got %d", len(rels))
	}
	if rels[0].RelationType != models.RelCausedBy {
		t.Errorf("tipo esperado 'caused_by', got '%s'", rels[0].RelationType)
	}
}

func TestSupersedesMarksInactive(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	oldID, _ := db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryDecision,
		Title: "Usar REST", Content: "API REST original",
	})
	newID, _ := db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryDecision,
		Title: "Usar GraphQL", Content: "Cambiamos a GraphQL",
	})

	// La relación supersedes debe marcar la vieja como inactiva
	db.CreateRelation(int(newID), int(oldID), models.RelSupersedes, nil)

	old, _, _ := db.GetObservation(int(oldID))
	if old.IsActive {
		t.Error("observación superseded debe tener is_active=false")
	}
	if old.SupersededBy == nil || *old.SupersededBy != int(newID) {
		t.Error("superseded_by debe apuntar a la nueva observación")
	}
}

// ─────────────────────────────────────────────
// User Requests
// ─────────────────────────────────────────────

func TestUserRequestLifecycle(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	// Crear petición
	reqID, err := db.SaveRequest(sessionID, &projectID, "Agregar dark mode", models.PriorityHigh)
	if err != nil {
		t.Fatalf("error guardando petición: %v", err)
	}

	// Debe aparecer en pendientes
	pending, _ := db.GetPendingRequests(&projectID, 10)
	if len(pending) != 1 {
		t.Fatalf("esperada 1 petición pendiente, got %d", len(pending))
	}
	if pending[0].Priority != models.PriorityHigh {
		t.Errorf("prioridad esperada 'high', got '%s'", pending[0].Priority)
	}

	// Actualizar a completada
	resolution := "Implementado con CSS variables"
	err = db.UpdateRequestStatus(int(reqID), models.RequestCompleted, &resolution)
	if err != nil {
		t.Fatalf("error actualizando petición: %v", err)
	}

	// Ya no debe estar en pendientes
	pending2, _ := db.GetPendingRequests(&projectID, 10)
	if len(pending2) != 0 {
		t.Errorf("no debería haber peticiones pendientes después de completar, got %d", len(pending2))
	}
}

func TestRequestPriorityOrder(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	// Crear peticiones con diferentes prioridades (en orden inverso)
	db.SaveRequest(sessionID, &projectID, "Tarea low", models.PriorityLow)
	db.SaveRequest(sessionID, &projectID, "Tarea critical", models.PriorityCritical)
	db.SaveRequest(sessionID, &projectID, "Tarea normal", models.PriorityNormal)

	pending, _ := db.GetPendingRequests(&projectID, 10)
	if len(pending) != 3 {
		t.Fatalf("esperadas 3 peticiones, got %d", len(pending))
	}

	// Critical debe estar primero
	if pending[0].Priority != models.PriorityCritical {
		t.Errorf("primera petición debe ser critical, got '%s'", pending[0].Priority)
	}
}

// ─────────────────────────────────────────────
// Context
// ─────────────────────────────────────────────

func TestGetContextCombinesScopes(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	// Global
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, Scope: models.ScopeGlobal,
		Category: models.CategoryPreference,
		Title:    "Preferencia global", Content: "Aplica a todo",
	})

	// Project
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryDecision,
		Title: "Decisión del proyecto", Content: "Solo aquí",
	})

	// Petición pendiente
	db.SaveRequest(sessionID, &projectID, "Agregar feature", models.PriorityNormal)

	ctx, err := db.GetContext(models.ContextInput{
		Project:         &projectID,
		MaxTokens:       4000,
		IncludeRequests: true,
	})
	if err != nil {
		t.Fatalf("error obteniendo contexto: %v", err)
	}

	if ctx.TotalItems < 3 {
		t.Errorf("contexto debe incluir al menos 3 items (session + obs + request), got %d", ctx.TotalItems)
	}
	if len(ctx.PendingRequests) != 1 {
		t.Errorf("debe haber 1 petición pendiente, got %d", len(ctx.PendingRequests))
	}
}

// ─────────────────────────────────────────────
// Stats
// ─────────────────────────────────────────────

func TestGetStats(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryBug,
		Title: "Bug 1", Content: "Contenido",
	})
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, ProjectID: &projectID,
		Scope: models.ScopeProject, Category: models.CategoryBug,
		Title: "Bug 2", Content: "Contenido",
	})
	db.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID, Scope: models.ScopeGlobal,
		Category: models.CategoryDecision,
		Title:    "Decisión global", Content: "Contenido",
	})

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("error obteniendo stats: %v", err)
	}

	if stats.TotalProjects != 1 {
		t.Errorf("esperado 1 proyecto, got %d", stats.TotalProjects)
	}
	if stats.TotalObservations != 3 {
		t.Errorf("esperadas 3 observaciones, got %d", stats.TotalObservations)
	}
	if stats.ByCategory["bug"] != 2 {
		t.Errorf("esperados 2 bugs, got %d", stats.ByCategory["bug"])
	}
	if stats.ByScope["global"] != 1 {
		t.Errorf("esperada 1 global, got %d", stats.ByScope["global"])
	}
}

// ─────────────────────────────────────────────
// Snapshots
// ─────────────────────────────────────────────

func TestSaveSnapshot(t *testing.T) {
	db := setupTestDB(t)

	projectID := "test-app"
	db.CreateProject(models.Project{ID: projectID, Name: "Test"})
	sessionID, _ := db.StartSession(&projectID, nil)

	rawLen := 5000
	id, err := db.SaveSnapshot(sessionID, "pre_compact", "Resumen antes de compactación", &rawLen)
	if err != nil {
		t.Fatalf("error guardando snapshot: %v", err)
	}
	if id == 0 {
		t.Fatal("snapshot ID debe ser > 0")
	}
}
