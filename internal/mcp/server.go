package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/florelmx/bitacora/internal/db"
)

// NewServer crea y configura el servidor MCP de Bitácora.
// Recibe una instancia de DB porque todas las herramientas necesitan
// acceso a la base de datos.
func NewServer(database *db.DB) *server.MCPServer {
	// Crear el servidor MCP con metadata.
	// Esta info la ve el agente cuando descubre las herramientas disponibles.
	s := server.NewMCPServer(
		"bitacora",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// Crear el handler que contiene la referencia a la DB.
	// Todos los tool handlers necesitan acceso a la DB,
	// así que los agrupamos en un struct.
	h := &handlers{db: database}

	// ── Registrar herramientas ──
	// Cada AddTool recibe: la definición (nombre + schema) y el handler.

	// bit_start_session
	s.AddTool(
		mcp.NewTool("bit_start_session",
			mcp.WithDescription("Iniciar sesión de trabajo. Registra inicio, detecta proyecto, y devuelve contexto reciente. LLAMAR AL INICIO DE CADA SESIÓN."),
			mcp.WithString("project_id", mcp.Description("ID del proyecto (slug o nombre del repo)"), mcp.Required()),
			mcp.WithString("project_name", mcp.Description("Nombre legible del proyecto")),
			mcp.WithString("project_path", mcp.Description("Ruta absoluta del proyecto")),
			mcp.WithString("git_remote", mcp.Description("URL del git remote origin")),
			mcp.WithString("workspace", mcp.Description("Nombre del workspace (para monorepos)")),
		),
		h.startSession,
	)

	// bit_end_session
	s.AddTool(
		mcp.NewTool("bit_end_session",
			mcp.WithDescription("Finalizar sesión activa. Genera resumen con objetivos, tareas y archivos. LLAMAR AL FINAL DE CADA SESIÓN."),
			mcp.WithString("session_id", mcp.Description("ID de la sesión a finalizar"), mcp.Required()),
			mcp.WithString("summary", mcp.Description("Resumen de lo logrado en la sesión")),
		),
		h.endSession,
	)

	// bit_save
	s.AddTool(
		mcp.NewTool("bit_save",
			mcp.WithDescription("Guardar observación en memoria. Categorías: decision (por qué X sobre Y), bug (problema + solución), pattern (patrón útil), note (contexto), preference (convención personal). GUARDAR PROACTIVAMENTE después de trabajo significativo."),
			mcp.WithString("session_id", mcp.Description("ID de la sesión activa"), mcp.Required()),
			mcp.WithString("category", mcp.Description("Tipo: decision, bug, pattern, note, request, preference"), mcp.Required()),
			mcp.WithString("title", mcp.Description("Título corto y buscable"), mcp.Required()),
			mcp.WithString("content", mcp.Description("Detalle en formato What/Why/Where/Learned"), mcp.Required()),
			mcp.WithString("scope", mcp.Description("Alcance: project (default), global, workspace")),
			mcp.WithString("project_id", mcp.Description("ID del proyecto (omitir para scope global)")),
		),
		h.saveObservation,
	)

	// bit_search
	s.AddTool(
		mcp.NewTool("bit_search",
			mcp.WithDescription("Búsqueda full-text en memoria. Usa FTS5 con ranking por relevancia. BUSCAR ANTES DE TOMAR DECISIONES para verificar si ya existe contexto previo."),
			mcp.WithString("query", mcp.Description("Texto a buscar (ej: 'bug autenticación', 'patrón cache')"), mcp.Required()),
			mcp.WithString("category", mcp.Description("Filtrar por categoría: decision, bug, pattern, note, preference")),
			mcp.WithString("project", mcp.Description("Filtrar por proyecto")),
			mcp.WithString("scope", mcp.Description("Filtrar por scope: global, project, workspace")),
			mcp.WithNumber("limit", mcp.Description("Máximo de resultados (default: 20)")),
		),
		h.search,
	)

	// bit_context
	s.AddTool(
		mcp.NewTool("bit_context",
			mcp.WithDescription("Obtener contexto completo en UNA sola llamada. Devuelve sesiones recientes, decisiones, bugs, patrones, notas y peticiones pendientes. Combina scopes global + project + workspace."),
			mcp.WithString("project", mcp.Description("ID del proyecto")),
			mcp.WithString("workspace", mcp.Description("Nombre del workspace")),
			mcp.WithBoolean("include_requests", mcp.Description("Incluir peticiones pendientes (default: true)")),
		),
		h.getContext,
	)

	// bit_get
	s.AddTool(
		mcp.NewTool("bit_get",
			mcp.WithDescription("Obtener observación completa por ID. Incluye contenido completo, relaciones y registra el acceso (reinforcement)."),
			mcp.WithNumber("id", mcp.Description("ID de la observación"), mcp.Required()),
		),
		h.getObservation,
	)

	// bit_save_request
	s.AddTool(
		mcp.NewTool("bit_save_request",
			mcp.WithDescription("Guardar petición del usuario. Las peticiones pendientes aparecen automáticamente en el contexto de próximas sesiones."),
			mcp.WithString("session_id", mcp.Description("ID de la sesión activa"), mcp.Required()),
			mcp.WithString("request", mcp.Description("Lo que pidió el usuario"), mcp.Required()),
			mcp.WithString("project_id", mcp.Description("ID del proyecto")),
			mcp.WithString("priority", mcp.Description("Prioridad: low, normal (default), high, critical")),
		),
		h.saveRequest,
	)

	// bit_update_request
	s.AddTool(
		mcp.NewTool("bit_update_request",
			mcp.WithDescription("Actualizar estado de una petición: pending → in_progress → completed/deferred."),
			mcp.WithNumber("id", mcp.Description("ID de la petición"), mcp.Required()),
			mcp.WithString("status", mcp.Description("Nuevo estado: pending, in_progress, completed, deferred"), mcp.Required()),
			mcp.WithString("resolution", mcp.Description("Cómo se resolvió (para completed)")),
		),
		h.updateRequest,
	)

	// bit_relate
	s.AddTool(
		mcp.NewTool("bit_relate",
			mcp.WithDescription("Crear relación entre dos observaciones. Tipos: caused_by, supersedes (marca la vieja como inactiva), relates_to, contradicts, depends_on, derived_from."),
			mcp.WithNumber("source_id", mcp.Description("ID de la observación origen"), mcp.Required()),
			mcp.WithNumber("target_id", mcp.Description("ID de la observación destino"), mcp.Required()),
			mcp.WithString("relation_type", mcp.Description("Tipo: caused_by, supersedes, relates_to, contradicts, depends_on, derived_from"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Descripción de la relación")),
		),
		h.createRelation,
	)

	// bit_stats
	s.AddTool(
		mcp.NewTool("bit_stats",
			mcp.WithDescription("Estadísticas del sistema de memoria. Contadores por categoría, scope, sesiones y proyectos."),
		),
		h.getStats,
	)

	// bit_list_sessions
	s.AddTool(
		mcp.NewTool("bit_list_sessions",
			mcp.WithDescription("Listar sesiones de trabajo. Filtrable por proyecto y estado."),
			mcp.WithString("project", mcp.Description("Filtrar por proyecto")),
			mcp.WithString("status", mcp.Description("Filtrar por estado: active, completed, compacted, abandoned")),
			mcp.WithNumber("limit", mcp.Description("Máximo de resultados (default: 10)")),
		),
		h.listSessions,
	)

	return s
}