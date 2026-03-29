package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Instrucciones que se agregan a CLAUDE.md
const claudeMDContent = `
## Memory (Bitácora)
You have access to persistent memory via MCP tools (bit_save, bit_search, bit_context, etc.).
- ALWAYS call bit_start_session at the beginning of each session.
- Save proactively after significant work — don't wait to be asked.
- Search memory with bit_search BEFORE making decisions to check for prior context.
- After any compaction or context reset, call bit_context to recover session state.
- At the end of a session, call bit_end_session with a summary.
`

// Run ejecuta el setup global o por proyecto
func Run(project bool) error {
	binaryPath, err := findBinary()
	if err != nil {
		return err
	}

	fmt.Printf("📍 Binario encontrado: %s\n", binaryPath)

	if project {
		return setupProject(binaryPath)
	}
	return setupGlobal(binaryPath)
}

// findBinary busca la ruta absoluta del binario bitacora
func findBinary() (string, error) {
	// Primero busca en el PATH del sistema
	path, err := exec.LookPath("bitacora")
	if err == nil {
		return filepath.Abs(path)
	}
	// Fallback: ruta del ejecutable actual
	exe, err := os.Executable()
	if err == nil {
		return filepath.Abs(exe)
	}
	return "", fmt.Errorf("no se encontró el binario de bitacora. Instálalo en tu PATH primero")
}

// setupGlobal configura ~/.claude/settings.json y ~/.claude/CLAUDE.md
func setupGlobal(binaryPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")

	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("error creando directorio .claude: %w", err)
	}

	if err := updateSettings(settingsPath, binaryPath); err != nil {
		return err
	}
	if err := updateClaudeMD(claudeMDPath); err != nil {
		return err
	}

	fmt.Println("✅ Configuración global completada:")
	fmt.Printf("   Settings: %s\n", settingsPath)
	fmt.Printf("   CLAUDE.md: %s\n", claudeMDPath)
	return nil
}

// setupProject configura .claude/settings.json y CLAUDE.md en el directorio actual
func setupProject(binaryPath string) error {
	claudeDir := ".claude"
	settingsPath := filepath.Join(claudeDir, "settings.json")
	claudeMDPath := "CLAUDE.md"

	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("error creando directorio .claude: %w", err)
	}

	if err := updateSettings(settingsPath, binaryPath); err != nil {
		return err
	}
	if err := updateClaudeMD(claudeMDPath); err != nil {
		return err
	}

	fmt.Println("✅ Configuración de proyecto completada:")
	fmt.Printf("   Settings: %s\n", settingsPath)
	fmt.Printf("   CLAUDE.md: %s\n", claudeMDPath)
	return nil
}

// updateSettings lee el settings.json existente y agrega MCP + hooks sin borrar lo demás
func updateSettings(path string, binaryPath string) error {
	settings := map[string]interface{}{}

	// Leer archivo existente (si hay)
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &settings)
	}

	// Agregar servidor MCP
	mcpServers, _ := settings["mcpServers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
	}
	mcpServers["bitacora"] = map[string]interface{}{
		"command": binaryPath,
		"args":    []string{"mcp"},
	}
	settings["mcpServers"] = mcpServers

	// Agregar hooks
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	hookEntry := func(cmd string) []interface{} {
		return []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": binaryPath + " " + cmd,
					},
				},
			},
		}
	}

	hooks["SessionStart"] = hookEntry("context")
	hooks["SessionEnd"] = hookEntry("end-session")
	hooks["PreCompact"] = hookEntry("end-session")
	settings["hooks"] = hooks

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando settings: %w", err)
	}

	return os.WriteFile(path, output, 0644)
}

// updateClaudeMD agrega instrucciones de Bitácora si no están presentes (idempotente)
func updateClaudeMD(path string) error {
	existing := ""
	data, err := os.ReadFile(path)
	if err == nil {
		existing = string(data)
	}

	// No duplicar si ya existe
	if strings.Contains(existing, "## Memory (Bitácora)") {
		fmt.Println("   CLAUDE.md ya contiene instrucciones de Bitácora (sin cambios)")
		return nil
	}

	content := existing + "\n" + claudeMDContent
	return os.WriteFile(path, []byte(content), 0644)
}