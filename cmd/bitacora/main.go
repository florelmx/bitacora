package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/florelmx/bitacora/internal/db"
	mcpserver "github.com/florelmx/bitacora/internal/mcp"
	"github.com/florelmx/bitacora/internal/models"
	"github.com/florelmx/bitacora/internal/setup"
)

var rootCmd = &cobra.Command{
	Use:   "bitacora",
	Short: "Ship's log for AI agents",
	Long:  "Bitácora — Persistent memory that survives between sessions.\nRepo: github.com/florelmx/bitacora",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Iniciar servidor MCP (stdio)",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		s := mcpserver.NewServer(database)
		if err := server.ServeStdio(s); err != nil {
			fmt.Fprintf(os.Stderr, "Error en servidor MCP: %v\n", err)
			os.Exit(1)
		}
	},
}

var contextProject string

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Imprimir contexto reciente del proyecto",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		var projectPtr *string
		if contextProject != "" {
			projectPtr = &contextProject
		}

		ctx, err := database.GetContext(models.ContextInput{
			Project:         projectPtr,
			MaxTokens:       4000,
			IncludeRequests: true,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("## Bitácora — Contexto del proyecto")
		fmt.Println()

		if len(ctx.RecentSessions) > 0 {
			fmt.Printf("### Sesiones recientes (%d)\n", len(ctx.RecentSessions))
			for _, s := range ctx.RecentSessions {
				summary := "sin resumen"
				if s.Summary != nil {
					summary = *s.Summary
				}
				fmt.Printf("- [%s] %s — %s\n", s.Status, s.ID, summary)
			}
			fmt.Println()
		}

		if len(ctx.Decisions) > 0 {
			fmt.Printf("### Decisiones recientes (%d)\n", len(ctx.Decisions))
			for _, o := range ctx.Decisions {
				fmt.Printf("- #%d: %s\n", o.ID, o.Title)
			}
			fmt.Println()
		}

		if len(ctx.Bugs) > 0 {
			fmt.Printf("### Bugs conocidos (%d)\n", len(ctx.Bugs))
			for _, o := range ctx.Bugs {
				fmt.Printf("- #%d: %s\n", o.ID, o.Title)
			}
			fmt.Println()
		}

		if len(ctx.Patterns) > 0 {
			fmt.Printf("### Patrones establecidos (%d)\n", len(ctx.Patterns))
			for _, o := range ctx.Patterns {
				fmt.Printf("- #%d: %s\n", o.ID, o.Title)
			}
			fmt.Println()
		}

		if len(ctx.Notes) > 0 {
			fmt.Printf("### Notas (%d)\n", len(ctx.Notes))
			for _, o := range ctx.Notes {
				fmt.Printf("- #%d: %s\n", o.ID, o.Title)
			}
			fmt.Println()
		}

		if len(ctx.Preferences) > 0 {
			fmt.Printf("### Preferencias (%d)\n", len(ctx.Preferences))
			for _, o := range ctx.Preferences {
				fmt.Printf("- #%d: %s\n", o.ID, o.Title)
			}
			fmt.Println()
		}

		if len(ctx.PendingRequests) > 0 {
			fmt.Printf("### Peticiones pendientes (%d)\n", len(ctx.PendingRequests))
			for _, r := range ctx.PendingRequests {
				fmt.Printf("- #%d [%s]: %s\n", r.ID, r.Priority, r.Request)
			}
			fmt.Println()
		}

		if ctx.TotalItems == 0 {
			fmt.Println("No hay contexto previo para este proyecto.")
		} else {
			fmt.Printf("Total: %d items de contexto cargados.\n", ctx.TotalItems)
		}
	},
}

var searchProject string
var searchCategory string
var searchLimit int

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Buscar en memoria",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		input := models.SearchInput{
			Query: args[0],
			Limit: searchLimit,
		}
		if searchProject != "" {
			input.Project = &searchProject
		}
		if searchCategory != "" {
			cat := models.Category(searchCategory)
			input.Category = &cat
		}

		results, err := database.Search(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(results) == 0 {
			fmt.Printf("No se encontraron resultados para '%s'\n", args[0])
			return
		}

		fmt.Printf("🔍 %d resultado(s) para '%s':\n\n", len(results), args[0])
		for _, r := range results {
			fmt.Printf("  #%d [%s] %s\n", r.Observation.ID, r.Observation.Category, r.Observation.Title)
			fmt.Printf("     scope: %s | score: %.2f\n", r.Observation.Scope, r.EffectiveScore)
			if len(r.Observation.Tags) > 0 {
				fmt.Printf("     tags: %v\n", r.Observation.Tags)
			}
			fmt.Println()
		}
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Mostrar estadísticas del sistema de memoria",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		stats, err := database.GetStats()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("📊 Bitácora — Estadísticas")
		fmt.Println()
		fmt.Printf("  Proyectos:      %d\n", stats.TotalProjects)
		fmt.Printf("  Sesiones:       %d total, %d activas\n", stats.TotalSessions, stats.ActiveSessions)
		fmt.Printf("  Observaciones:  %d total, %d activas\n", stats.TotalObservations, stats.ActiveObservations)
		fmt.Printf("  Relaciones:     %d\n", stats.TotalRelations)
		fmt.Printf("  Peticiones:     %d total, %d pendientes\n", stats.TotalRequests, stats.PendingRequests)
		fmt.Println()

		if len(stats.ByCategory) > 0 {
			fmt.Println("  Por categoría:")
			for cat, count := range stats.ByCategory {
				fmt.Printf("    %-12s %d\n", cat, count)
			}
			fmt.Println()
		}

		if len(stats.ByScope) > 0 {
			fmt.Println("  Por scope:")
			for scope, count := range stats.ByScope {
				fmt.Printf("    %-12s %d\n", scope, count)
			}
		}
	},
}

var endSessionID string

var endSessionCmd = &cobra.Command{
	Use:   "end-session",
	Short: "Finalizar sesión activa",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		if endSessionID != "" {
			err = database.EndSession(endSessionID, "Sesión finalizada via CLI.", nil, nil)
		} else {
			session, err2 := database.GetActiveSession(nil)
			if err2 != nil || session == nil {
				fmt.Println("No hay sesiones activas.")
				return
			}
			err = database.EndSession(session.ID, "Sesión finalizada automáticamente.", nil, nil)
			endSessionID = session.ID
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Sesión %s finalizada.\n", endSessionID)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Mostrar versión de Bitácora",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Bitácora v0.3.0")
	},
}

var setupProject bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configurar Claude Code (MCP + hooks + CLAUDE.md)",
	Run: func(cmd *cobra.Command, args []string) {
		if err := setup.Run(setupProject); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	contextCmd.Flags().StringVarP(&contextProject, "project", "p", "", "Filtrar por proyecto")

	searchCmd.Flags().StringVarP(&searchProject, "project", "p", "", "Filtrar por proyecto")
	searchCmd.Flags().StringVarP(&searchCategory, "category", "c", "", "Filtrar por categoría")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 20, "Máximo de resultados")

	endSessionCmd.Flags().StringVarP(&endSessionID, "session", "s", "", "ID de sesión a finalizar")

	setupCmd.Flags().BoolVar(&setupProject, "project", false, "Configurar a nivel de proyecto en vez de global")

	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(endSessionCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(setupCmd)

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
