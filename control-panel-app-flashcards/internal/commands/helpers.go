package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func connectDB(ctx context.Context, dbURL string) (*pgx.Conn, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	return pgx.Connect(ctx, dbURL)
}

func loadCards(path string) ([]Flashcard, error) {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cards.json: %w", err)
	}
	var cards []Flashcard
	if err := json.Unmarshal(raw, &cards); err != nil {
		return nil, fmt.Errorf("invalid cards json: %w", err)
	}
	return cards, nil
}

func getOrCreateTagIDs(conn *pgx.Conn, ctx context.Context, cards []Flashcard) (map[string]int, error) {
	tagIDs := make(map[string]int)
	rows, err := conn.Query(ctx, `SELECT id, name FROM tags`)
	if err != nil {
		return nil, fmt.Errorf("query existing tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan tag row: %w", err)
		}
		tagIDs[strings.ToLower(name)] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	for _, card := range cards {
		for _, tag := range card.Tags {
			key := strings.ToLower(tag)
			if _, exists := tagIDs[key]; !exists {
				var newID int
				if err := conn.QueryRow(ctx, `INSERT INTO tags (name) VALUES ($1) RETURNING id`, tag).Scan(&newID); err != nil {
					return nil, fmt.Errorf("insert tag %q: %w", tag, err)
				}
				tagIDs[key] = newID
			}
		}
	}
	return tagIDs, nil
}

func uploadAllImagesToSupabase(ctx context.Context, imagesDir string, supabaseURL string, apiKey string) error {
	imagesDirClean := filepath.Clean(imagesDir)
	bucket := "flashcard-assets"

	files, err := os.ReadDir(imagesDirClean)
	if err != nil {
		return fmt.Errorf("failed to read images directory %s: %w", imagesDirClean, err)
	}

	var failed int
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		localPath := filepath.Join(imagesDirClean, file.Name())
		uploadPath := file.Name() // flat structure in bucket

		f, err := os.Open(localPath)
		if err != nil {
			log.Printf("Failed to open %s: %v", localPath, err)
			failed++
			continue
		}

		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			log.Printf("Failed to read %s: %v", localPath, err)
			failed++
			continue
		}

		contentType := mime.TypeByExtension(filepath.Ext(file.Name()))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		url := fmt.Sprintf("%s/storage/v1/object/%s/%s", strings.TrimRight(supabaseURL, "/"), bucket, uploadPath)
		req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
		if err != nil {
			log.Printf("Failed to create request for %s: %v", uploadPath, err)
			failed++
			continue
		}
		// include both headers; anon key may be accepted via apikey header depending on project rules
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("apikey", apiKey)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("x-upsert", "true") // replace if exists

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("Failed to upload %s: %v", uploadPath, err)
			failed++
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 300 {
			log.Printf("Failed to upload %s: status %d, body: %s", uploadPath, resp.StatusCode, strings.TrimSpace(string(body)))
			failed++
			continue
		}

		log.Printf("Uploaded %s to bucket %s", uploadPath, bucket)
	}

	if failed > 0 {
		return fmt.Errorf("%d image upload(s) failed", failed)
	}
	return nil
}

func assignDefaultCardsToAllStudents(conn *pgx.Conn, ctx context.Context, cards []Flashcard) error {
	// Gather default card IDs from provided cards slice
	var defaultCardIDs []string
	for _, card := range cards {
		if card.Default {
			defaultCardIDs = append(defaultCardIDs, card.ID)
		}
	}
	if len(defaultCardIDs) == 0 {
		log.Println("No default cards to assign.")
		return nil
	}

	// Get all student IDs
	rows, err := conn.Query(ctx, `SELECT student_id FROM users_students`)
	if err != nil {
		return fmt.Errorf("fetch students: %w", err)
	}
	defer rows.Close()

	var studentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan student id: %w", err)
		}
		studentIDs = append(studentIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("students rows error: %w", err)
	}

	now := time.Now().Unix()
	for _, studentID := range studentIDs {
		for _, cardID := range defaultCardIDs {
			if _, err := conn.Exec(ctx, `
                INSERT INTO students_cards (student_id, card_id, due, status)
                VALUES ($1, $2, $3, 0)
                ON CONFLICT (student_id, card_id) DO NOTHING
            `, studentID, cardID, now); err != nil {
				// log and continue assigning others
				log.Printf("Failed to assign card %s to student %s: %v", cardID, studentID, err)
			}
		}
	}
	log.Printf("Assigned %d default cards to %d students.", len(defaultCardIDs), len(studentIDs))
	return nil
}

func clearCardData(conn *pgx.Conn, ctx context.Context) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// ensure rollback if commit doesn't happen
	defer func() { _ = tx.Rollback(ctx) }()

	stmts := []string{
		`DELETE FROM cards_tags`,
		`DELETE FROM students_cards`,
		`DELETE FROM cards`,
	}

	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s); err != nil {
			return fmt.Errorf("failed to exec %q: %w", s, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func insertCards(conn *pgx.Conn, ctx context.Context, cards []Flashcard, tagIDs map[string]int, userID ...string) error {
	user := "info"
	if len(userID) > 0 && userID[0] != "" {
		user = userID[0]
	}

	for _, card := range cards {
		assetsJSON, err := json.Marshal(card.Assets)
		if err != nil {
			return fmt.Errorf("failed to marshal assets for card %s: %w", card.ID, err)
		}

		if _, err := conn.Exec(ctx,
			`INSERT INTO cards (id, front, back, assets, created_by)
             VALUES ($1, $2, $3, $4, NULL)`,
			card.ID, card.Front, card.Back, assetsJSON,
		); err != nil {
			return fmt.Errorf("failed to insert card %s: %w", card.ID, err)
		}

		for _, tag := range card.Tags {
			tagID, ok := tagIDs[strings.ToLower(tag)]
			if !ok {
				return fmt.Errorf("tag ID for '%s' missing in cache", tag)
			}
			if _, err := conn.Exec(ctx,
				`INSERT INTO cards_tags (card_id, tag_id)
                 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				card.ID, tagID,
			); err != nil {
				return fmt.Errorf("failed to link card %s with tag %s: %w", card.ID, tag, err)
			}
		}

		if _, err := conn.Exec(ctx,
			`INSERT INTO students_cards (student_id, card_id, due, status)
             VALUES ($1, $2, extract(epoch from now()), 0)
             ON CONFLICT DO NOTHING`,
			user, card.ID,
		); err != nil {
			return fmt.Errorf("failed to add card %s to user '%s': %w", card.ID, user, err)
		}
	}

	return nil
}

func assignUserStudent(conn *pgx.Conn, ctx context.Context, userID string, studentID string) error {
	_, err := conn.Exec(ctx, `
        INSERT INTO users_students (user_id, student_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
    `, userID, studentID)
	return err
}

// upsertCard inserts or updates a card row.
func upsertCard(conn *pgx.Conn, ctx context.Context, card Flashcard) error {
	assetsJSON, err := json.Marshal(card.Assets)
	if err != nil {
		return fmt.Errorf("marshal assets: %w", err)
	}
	_, err = conn.Exec(ctx, `
        INSERT INTO cards (id, front, back, assets, created_by)
        VALUES ($1, $2, $3, $4, NULL)
        ON CONFLICT (id) DO UPDATE
        SET front = EXCLUDED.front,
            back = EXCLUDED.back,
            assets = EXCLUDED.assets,
            created_by = NULL
    `, card.ID, card.Front, card.Back, assetsJSON)
	if err != nil {
		return fmt.Errorf("exec upsert: %w", err)
	}
	return nil
}

// pruneTagsForCard deletes cards_tags links for a card that are not in desiredLower.
func pruneTagsForCard(conn *pgx.Conn, ctx context.Context, cardID string, desiredLower []string) error {
	_, err := conn.Exec(ctx, `
        DELETE FROM cards_tags
        WHERE card_id = $1
          AND tag_id NOT IN (
            SELECT id FROM tags WHERE LOWER(name) = ANY($2)
          )
    `, cardID, desiredLower)
	if err != nil {
		return fmt.Errorf("prune tags exec: %w", err)
	}
	return nil
}

// ensureCardTagLinks inserts any missing cards_tags links (idempotent).
func ensureCardTagLinks(conn *pgx.Conn, ctx context.Context, card Flashcard, tagIDs map[string]int) error {
	for _, tag := range card.Tags {
		tagID, ok := tagIDs[strings.ToLower(tag)]
		if !ok {
			return fmt.Errorf("missing tag id for %q", tag)
		}
		if _, err := conn.Exec(ctx, `
            INSERT INTO cards_tags (card_id, tag_id)
            VALUES ($1, $2)
            ON CONFLICT DO NOTHING
        `, card.ID, tagID); err != nil {
			return fmt.Errorf("insert cards_tags: %w", err)
		}
	}
	return nil
}
