package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/florelmx/bitacora/internal/db"
	"github.com/florelmx/bitacora/internal/models"
)

func main() {
	// Abrir base de datos
	database, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error abriendo Bitácora: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	fmt.Println("✅ Bitácora inicializada")
	fmt.Printf("📁 Base de datos: %s\n\n", database.Path())

	// ── 1. Crear proyecto ──
	projectID := "mi-app"
	path := "/Users/leonardoflores/projects/mi-app"
	remote := "github.com/florelmx/mi-app"

	err = database.CreateProject(models.Project{
		ID:        projectID,
		Name:      "Mi App",
		Path:      &path,
		GitRemote: &remote,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creando proyecto: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("📂 Proyecto creado: mi-app")

	// ── 2. Iniciar sesión ──
	sessionID, err := database.StartSession(&projectID, []string{"Implementar auth", "Corregir bugs"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error iniciando sesión: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("🟢 Sesión iniciada: %s\n\n", sessionID)

	// ── 3. Guardar observaciones ──
	obsID1, _ := database.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryDecision,
		Title:     "Usar JWT para autenticación",
		Content:   "What: Elegimos JWT sobre sessions. Why: La API es stateless y necesitamos escalabilidad horizontal. Where: src/auth/jwt.ts",
		Tags:      []string{"auth", "jwt", "backend"},
		Files:     []string{"src/auth/jwt.ts"},
	})
	fmt.Printf("🧠 Decisión guardada: #%d\n", obsID1)

	obsID2, _ := database.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		ProjectID: &projectID,
		Scope:     models.ScopeProject,
		Category:  models.CategoryBug,
		Title:     "Race condition en refresh de tokens",
		Content:   "What: Dos requests simultáneos con token expirado causan doble refresh. Why: No hay mutex en el interceptor. Where: src/auth/interceptor.ts. Learned: Usar mutex para operaciones de refresh.",
		Tags:      []string{"auth", "bug", "concurrency"},
		Files:     []string{"src/auth/interceptor.ts"},
	})
	fmt.Printf("🐛 Bug guardado: #%d\n", obsID2)

	// Observación global (aplica a todos los proyectos)
	obsID3, _ := database.SaveObservation(models.SaveObservationInput{
		SessionID: sessionID,
		Scope:     models.ScopeGlobal,
		Category:  models.CategoryPreference,
		Title:     "Siempre usar conventional commits",
		Content:   "What: Formato de commits: feat:, fix:, docs:, etc. Why: Consistencia y changelogs automáticos.",
		Tags:      []string{"git", "conventions"},
	})
	fmt.Printf("🌍 Preferencia global guardada: #%d\n\n", obsID3)

	// ── 4. Crear relación ──
	desc := "La decisión de JWT se tomó porque encontramos el bug de race condition"
	database.CreateRelation(int(obsID1), int(obsID2), models.RelCausedBy, &desc)
	fmt.Printf("🔗 Relación creada: #%d -[caused_by]-> #%d\n\n", obsID1, obsID2)

	// ── 5. Buscar con FTS5 ──
	fmt.Println("🔍 Buscando 'autenticación JWT'...")
	results, err := database.Search(models.SearchInput{
		Query: "autenticación JWT",
		Limit: 10,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error en búsqueda: %v\n", err)
	} else {
		fmt.Printf("   Encontrados: %d resultados\n", len(results))
		for _, r := range results {
			fmt.Printf("   → [%s] %s (score: %.2f)\n", r.Observation.Category, r.Observation.Title, r.EffectiveScore)
		}
	}

	fmt.Println()
	fmt.Println("🔍 Buscando 'race condition'...")
	results2, _ := database.Search(models.SearchInput{
		Query: "race condition",
		Limit: 10,
	})
	fmt.Printf("   Encontrados: %d resultados\n", len(results2))
	for _, r := range results2 {
		fmt.Printf("   → [%s] %s (score: %.2f)\n", r.Observation.Category, r.Observation.Title, r.EffectiveScore)
	}

	// ── 6. Guardar petición del usuario ──
	fmt.Println()
	reqID, _ := database.SaveRequest(sessionID, &projectID, "Agregar dark mode al dashboard", models.PriorityHigh)
	fmt.Printf("📋 Petición guardada: #%d (prioridad: high)\n", reqID)

	// ── 7. Obtener contexto completo ──
	fmt.Println()
	fmt.Println("📦 Cargando contexto completo del proyecto...")
	ctx, _ := database.GetContext(models.ContextInput{
		Project:         &projectID,
		MaxTokens:       4000,
		IncludeRequests: true,
	})
	fmt.Printf("   Sesiones recientes: %d\n", len(ctx.RecentSessions))
	fmt.Printf("   Decisiones: %d\n", len(ctx.Decisions))
	fmt.Printf("   Bugs: %d\n", len(ctx.Bugs))
	fmt.Printf("   Patrones: %d\n", len(ctx.Patterns))
	fmt.Printf("   Notas: %d\n", len(ctx.Notes))
	fmt.Printf("   Peticiones pendientes: %d\n", len(ctx.PendingRequests))
	fmt.Printf("   Total items: %d\n", ctx.TotalItems)

	// ── 8. Stats ──
	fmt.Println()
	stats, _ := database.GetStats()
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Printf("📊 Estadísticas:\n%s\n", string(statsJSON))

	// ── 9. Finalizar sesión ──
	fmt.Println()
	err = database.EndSession(sessionID, "Implementamos JWT auth, encontramos race condition en tokens, establecimos conventional commits.", []string{"JWT auth", "Fix race condition"}, []string{"src/auth/jwt.ts", "src/auth/interceptor.ts"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finalizando sesión: %v\n", err)
	} else {
		fmt.Println("✅ Sesión finalizada con resumen")
	}

	fmt.Println()
	fmt.Println("🎉 Bitácora v0.1 — Core funcionando correctamente")
}