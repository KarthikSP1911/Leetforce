// Command contestscore prints the standings of one contest as JSON, computed
// by contest.Score from the real verdicts in Postgres. The mock-contest script
// uses it to check scores; DATABASE_URL names the database.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"leetforce/api/internal/contest"
	"leetforce/api/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "contestscore:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: contestscore <contest-slug>")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, url)
	if err != nil {
		return err
	}
	defer db.Close()
	pg := contest.NewPG(db.Pool())
	c, err := pg.GetContest(ctx, os.Args[1], "")
	if err != nil {
		return fmt.Errorf("contest %q: %w", os.Args[1], err)
	}
	events, err := pg.Events(ctx, c.ID)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(contest.Score(events))
}
