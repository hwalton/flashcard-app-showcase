package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"github.com/abstract-tutoring/app-flashcards/control-panel/internal/commands"
)

func usage() {
	fmt.Println("go run main.go <command> [args...]")
}

type cmdHandler func([]string) error

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		fmt.Fprintln(os.Stderr, "no .env file found — relying on environment")
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	handlers := map[string]cmdHandler{
		"reset-cards-dev":     handleResetCardsDev,
		"reset-db-dev":        handleResetDBDev,
		"sync-official-cards": handleSyncOfficialCards,
		"run-migrations-up":   handleRunMigrationsUp,
		"exec-sql":            handleExecSQL, // use the custom handler so we can inject SQL input
		"assign-all-cards":    handleAssignAll,
		"backup-supabase":     handleBackupSupabase,
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	handler, ok := handlers[cmd]
	if !ok {
		usage()
		os.Exit(2)
	}

	if err := handler(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", cmd, err)
		os.Exit(1)
	}
}

func handleResetCardsDev(args []string) error {
	return commands.ResetCardsDev()
}

func handleResetDBDev(args []string) error {
	return commands.ResetDBDev()
}

func handleSyncOfficialCards(args []string) error {
	var isProd bool
	if len(args) >= 1 {
		if args[0] == "--dev" || args[0] == "-d" {
			isProd = false
		} else if args[0] == "--prod" || args[0] == "-p" {
			isProd = true
		} else {
			return fmt.Errorf("Must provide argument --dev or --prod")
		}
	}
	return commands.SyncOfficialCards(isProd)
}

func handleRunMigrationsUp(args []string) error {
	var isProd bool
	if len(args) >= 1 {
		if args[0] == "--dev" || args[0] == "-d" {
			isProd = false
		} else if args[0] == "--prod" || args[0] == "-p" {
			isProd = true
		} else {
			return fmt.Errorf("Must provide argument --dev or --prod")
		}
	}

	return commands.RunMigrationsUp(isProd)
}

func handleExecSQL(args []string) error {
	var isProd bool
	if len(args) >= 1 {
		if args[0] == "--dev" || args[0] == "-d" {
			isProd = false
		} else if args[0] == "--prod" || args[0] == "-p" {
			isProd = true
		} else {
			return fmt.Errorf("must provide argument --dev or --prod")
		}
	} else {
		return fmt.Errorf("must provide argument --dev or --prod")
	}

	// skip optional "--" separator if present
	pos := 1
	if pos < len(args) && args[pos] == "--" {
		pos++
	}

	if pos >= len(args) {
		return fmt.Errorf("no SQL provided; pass SQL as a positional argument after the env flag")
	}

	// join remaining args to preserve whitespace/newlines if shell split them
	sqlInput := strings.Join(args[pos:], " ")

	return commands.ExecSQL(sqlInput, isProd)
}

func handleAssignAll(args []string) error {
	var isProd bool
	if len(args) >= 1 {
		if args[0] == "--dev" || args[0] == "-d" {
			isProd = false
		} else if args[0] == "--prod" || args[0] == "-p" {
			isProd = true
		} else {
			return fmt.Errorf("must provide argument --dev or --prod")
		}
	} else {
		return fmt.Errorf("must provide argument --dev or --prod")
	}

	// skip optional "--" separator if present
	pos := 1
	if pos < len(args) && args[pos] == "--" {
		pos++
	}

	if pos >= len(args) {
		return fmt.Errorf("no student id provided; pass the student id as a positional argument after the env flag")
	}

	studentID := strings.Join(args[pos:], " ")
	// call the commands package function (signature expects student_id then isProd)
	return commands.AssignAllCards(studentID, isProd)
}

func handleBackupSupabase(args []string) error {
	var isProd bool
	if len(args) >= 1 {
		if args[0] == "--dev" || args[0] == "-d" {
			isProd = false
		} else if args[0] == "--prod" || args[0] == "-p" {
			isProd = true
		} else {
			return fmt.Errorf("must provide argument --dev or --prod")
		}
	} else {
		return fmt.Errorf("must provide argument --dev or --prod")
	}

	return commands.BackupSupabase(isProd)
}
