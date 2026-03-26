package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// El _ antes del import es un "blank import".
	// Importa el paquete SOLO por sus efectos secundarios (side effects).
	// Este paquete se registra como driver de SQLite al importarse.
	// Sin esta línea, sql.Open("sqlite", ...) fallaría porque
	// Go no sabría qué driver usar.
	_ "modernc.org/sqlite"
)

const DefaultDBDir = ".bitacora"
const DBFileName = "memory.db"

// DB es el struct principal. Envuelve la conexión de SQLite.
type DB struct {
	conn *sql.DB // puntero a la conexión (viene del paquete database/sql)
	path string  // ruta al archivo .db
}

// GetDBPath determina dónde guardar la base de datos.
// Prioridad: BITACORA_DIR (env var) > ~/.bitacora/
func GetDBPath() (string, error) {
	dir := os.Getenv("BITACORA_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no se pudo obtener home directory: %w", err)
		}
		dir = filepath.Join(home, DefaultDBDir)
	}

	// Crear directorio si no existe (como mkdir -p)
	// 0755 = permisos Unix: owner rwx, group rx, others rx
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("no se pudo crear directorio %s: %w", dir, err)
	}

	return filepath.Join(dir, DBFileName), nil
}

// Open abre o crea la base de datos en la ubicación por defecto.
func Open() (*DB, error) {
	dbPath, err := GetDBPath()
	if err != nil {
		return nil, err
	}
	return OpenAt(dbPath)
}

// OpenAt abre la base de datos en una ruta específica.
// Útil para tests: puedes pasarle un archivo temporal.
func OpenAt(dbPath string) (*DB, error) {
	// sql.Open prepara el driver pero NO abre la conexión todavía.
	// La conexión real se abre en el primer query o en Ping().
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir SQLite en %s: %w", dbPath, err)
	}

	// Ping() fuerza la conexión real — verifica que funciona
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("no se pudo conectar a SQLite: %w", err)
	}

	// PRAGMAs: configuran el motor de SQLite
	pragmas := []string{
		"PRAGMA journal_mode = WAL",     // Lecturas concurrentes sin bloqueo
		"PRAGMA synchronous = NORMAL",    // Balance velocidad/seguridad
		"PRAGMA foreign_keys = ON",       // Activar validación de FKs
		"PRAGMA cache_size = -64000",     // 64MB de cache en memoria
		"PRAGMA busy_timeout = 5000",     // Esperar 5s si DB bloqueada
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			conn.Close()
			return nil, fmt.Errorf("error ejecutando %s: %w", pragma, err)
		}
	}

	// Crear instancia de DB
	// &DB{...} crea el struct y devuelve un puntero a él.
	db := &DB{
		conn: conn,
		path: dbPath,
	}

	// Inicializar tablas (IF NOT EXISTS — seguro de correr múltiples veces)
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("error inicializando schema: %w", err)
	}

	return db, nil
}

// Close cierra la conexión. En Go los recursos se cierran explícitamente.
// El patrón típico de uso es:
//
//   db, err := db.Open()
//   if err != nil { log.Fatal(err) }
//   defer db.Close()
//
// "defer" ejecuta Close() cuando la función actual termine, SIEMPRE,
// incluso si hay un error o un return temprano. Es como un finally{}
// pero más elegante — lo escribes junto al Open, no al final.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn expone la conexión para queries directos.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Path devuelve la ruta al archivo de la base de datos.
func (db *DB) Path() string {
	return db.path
}