package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/florelmx/bitacora/internal/db"
	mcpserver "github.com/florelmx/bitacora/internal/mcp"
)

func main() {
	// Si el primer argumento es "mcp", arrancamos el servidor MCP
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runMCP()
		return
	}

	// Sin argumentos: mostrar info
	fmt.Println("🧭 Bitácora — Ship's log for AI agents")
	fmt.Println("   Persistent memory that survives between sessions")
	fmt.Println()
	fmt.Println("Uso:")
	fmt.Println("   bitacora mcp       Iniciar servidor MCP (stdio)")
	fmt.Println("   bitacora context   Imprimir contexto reciente")
	fmt.Println("   bitacora search    Buscar en memoria")
	fmt.Println("   bitacora stats     Mostrar estadísticas")
	fmt.Println("   bitacora setup     Configurar Claude Code")
	fmt.Println()
	fmt.Println("Repo: github.com/florelmx/bitacora")
}

func runMCP() {
	// Abrir DB
	database, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error abriendo Bitácora: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Crear servidor MCP con todas las herramientas registradas
	s := mcpserver.NewServer(database)

	// Iniciar servidor en modo stdio.
	// Esto bloquea — lee de stdin y escribe a stdout indefinidamente.
	// Claude Code lanza este proceso y se comunica con él por stdio.
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Error en servidor MCP: %v\n", err)
		os.Exit(1)
	}
}