package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hwalton/gdrivetoolbox/auth"
	"github.com/hwalton/gdrivetoolbox/deploy"

	"github.com/hwalton/psqltoolbox"
)

func ResetCardsDev() error {
	// load env vars
	var dbURL string
	var ok bool

	dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
	if !ok || dbURL == "" {
		return fmt.Errorf("DEV_SUPABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	userID, ok := os.LookupEnv("DEV_USER_ID")
	if !ok || userID == "" {
		return fmt.Errorf("DEV_USER_ID not set")
	}

	studentID, ok := os.LookupEnv("DEV_STUDENT_ID")
	if !ok || studentID == "" {
		return fmt.Errorf("DEV_STUDENT_ID not set")
	}

	if err := assignUserStudent(conn, ctx, userID, studentID); err != nil {
		return fmt.Errorf("assign user/student: %w", err)
	}

	cards, err := loadCards("../../../cards/cards.json")
	if err != nil {
		return fmt.Errorf("load cards: %w", err)
	}

	if err := clearCardData(conn, ctx); err != nil {
		return fmt.Errorf("failed to clear card data: %w", err)
	}

	tagIDs, err := getOrCreateTagIDs(conn, ctx, cards)
	if err != nil {
		return fmt.Errorf("get or create tag IDs: %w", err)
	}

	assignUser := "info"
	if err := insertCards(conn, ctx, cards, tagIDs, assignUser); err != nil {
		return err
	}

	if err := assignDefaultCardsToAllStudents(conn, ctx, cards); err != nil {
		return fmt.Errorf("assign default cards: %w", err)
	}

	var supabaseURL, apiKey string

	supabaseURL, ok = os.LookupEnv("DEV_NEXT_PUBLIC_SUPABASE_URL")
	if !ok || supabaseURL == "" {
		return fmt.Errorf("DEV_NEXT_PUBLIC_SUPABASE_URL not set")
	}
	apiKey, ok = os.LookupEnv("DEV_SUPABASE_SERVICE_ROLE_KEY")
	if !ok || apiKey == "" {
		apiKey, ok = os.LookupEnv("DEV_NEXT_PUBLIC_SUPABASE_ANON_KEY")
		if !ok || apiKey == "" {
			return fmt.Errorf("missing DEV_SUPABASE_SERVICE_ROLE_KEY and DEV_NEXT_PUBLIC_SUPABASE_ANON_KEY in environment")
		}
	}

	imagesDir := "../../../cards/images"

	if err := uploadAllImagesToSupabase(ctx, imagesDir, supabaseURL, apiKey); err != nil {
		return fmt.Errorf("upload images: %w", err)
	}

	fmt.Printf("replaced %d cards (with tags and assets) in dev\n", len(cards))
	return nil
}

func ResetDBDev() error {
	// load env vars
	var dbURL string
	var ok bool

	dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
	if !ok || dbURL == "" {
		return fmt.Errorf("DEV_SUPABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	migrationsPath := "../../../migrations"

	// drop all tables and optionally run migrations
	if err := psqltoolbox.DropTablesAndMigrate(ctx, conn, dbURL, migrationsPath); err != nil {
		return err
	}

	return nil
}

func SyncOfficialCards(isProd bool) error {
	var dbURL string
	var ok bool

	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	cards, err := loadCards("../../../cards/cards.json")
	if err != nil {
		return fmt.Errorf("load cards: %w", err)
	}

	tagIDs, err := getOrCreateTagIDs(conn, ctx, cards)
	if err != nil {
		return fmt.Errorf("get or create tag IDs: %w", err)
	}

	// Upsert cards and reconcile tags/links using helpers
	for _, card := range cards {
		if err := upsertCard(conn, ctx, card); err != nil {
			return fmt.Errorf("upsert card %s: %w", card.ID, err)
		}

		desiredLower := make([]string, 0, len(card.Tags))
		for _, t := range card.Tags {
			desiredLower = append(desiredLower, strings.ToLower(t))
		}
		if err := pruneTagsForCard(conn, ctx, card.ID, desiredLower); err != nil {
			return fmt.Errorf("prune tags for card %s: %w", card.ID, err)
		}

		if err := ensureCardTagLinks(conn, ctx, card, tagIDs); err != nil {
			return fmt.Errorf("link tags for card %s: %w", card.ID, err)
		}
	}

	fmt.Printf("Safely synced %d official cards without breaking student assignments.\n", len(cards))
	return nil
}

func RunMigrationsUp(isProd bool) error {
	var dbURL string
	var ok bool

	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
	}

	migrationsPath := "../../../migrations"

	fmt.Printf("[%s] Running DB migrations from %s...\n", time.Now().Format(time.RFC3339), migrationsPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "migrate", "-database", dbURL, "-path", migrationsPath, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("migrate up failed: %w", err)
	}

	fmt.Printf("[%s] Migrations applied.\n", time.Now().Format(time.RFC3339))
	return nil
}

func ExecSQL(sqlInput string, isProd bool) error {
	var dbURL string
	var ok bool

	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
	}

	// connect to db
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	// execute the SQL command
	if _, err := conn.Exec(ctx, sqlInput); err != nil {
		return fmt.Errorf("execute SQL: %w", err)
	}

	fmt.Println("SQL executed successfully.")
	return nil
}

func AssignAllCards(studentID string, isProd bool) error {
	var dbURL string
	var ok bool

	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	// assign all cards to the specified student
	cmdTag, err := conn.Exec(ctx, `
        INSERT INTO students_cards (student_id, card_id)
        SELECT $1, id FROM cards
        ON CONFLICT (student_id, card_id) DO NOTHING
    `, studentID)
	if err != nil {
		return fmt.Errorf("assign cards to '%s': %w", studentID, err)
	}
	rows := cmdTag.RowsAffected()
	fmt.Printf("Assigned %d card assignments to student '%s' (existing assignments left intact)\n", rows, studentID)

	return nil
}

func BackupSupabase(isProd bool) error {
	var dbURL string
	var ok bool
	var driveFolder string

	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
		driveFolder, ok = os.LookupEnv("PROD_SUPABASE_GDRIVE_BACKUP_FOLDER_ID")
		if !ok || driveFolder == "" {
			return fmt.Errorf("PROD_SUPABASE_GDRIVE_BACKUP_FOLDER_ID not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
		driveFolder, ok = os.LookupEnv("DEV_SUPABASE_GDRIVE_BACKUP_FOLDER_ID")
		if !ok || driveFolder == "" {
			return fmt.Errorf("DEV_SUPABASE_GDRIVE_BACKUP_FOLDER_ID not set")
		}
	}

	clientID, ok := os.LookupEnv("GOOGLE_CLIENT_ID")
	if !ok || clientID == "" {
		return fmt.Errorf("GOOGLE_CLIENT_ID not set")
	}

	clientSecret, ok := os.LookupEnv("GOOGLE_CLIENT_SECRET")
	if !ok || clientSecret == "" {
		return fmt.Errorf("GOOGLE_CLIENT_SECRET not set")
	}

	refreshToken, ok := os.LookupEnv("GOOGLE_REFRESH_TOKEN")
	if !ok || refreshToken == "" {
		return fmt.Errorf("GOOGLE_REFRESH_TOKEN not set")
	}

	// create backup filename
	backupFile := fmt.Sprintf("backup_%s.dump", time.Now().Format("2006-01-02_15-04-05"))

	// run pg_dump with timeout (uses helper in psqltoolbox)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if err := psqltoolbox.PgDumpToFile(ctx, dbURL, backupFile, 15*time.Minute); err != nil {
		_ = os.Remove(backupFile)
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	defer func() { _ = os.Remove(backupFile) }()

	accessToken, err := auth.GetGoogleAccessToken(clientID, clientSecret, refreshToken)
	if err != nil {
		return fmt.Errorf("get drive access token: %w", err)
	}

	fileID, err := deploy.UploadFileToDrive(accessToken, driveFolder, backupFile)
	if err != nil {
		return fmt.Errorf("upload to drive: %w", err)
	}

	fmt.Printf("Backup uploaded to Google Drive with file ID: %s\n", fileID)
	return nil
}
