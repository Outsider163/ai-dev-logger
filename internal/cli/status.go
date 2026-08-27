package cli

import (
	"fmt"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show note and embedding index status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(configPath)
		if err != nil {
			return err
		}
		model := strings.TrimSpace(cfg.LLM.EmbeddingModel)
		if model == "" {
			return fmt.Errorf("embedding model is empty, run config set --embedding-model")
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		status, err := db.GetEmbeddingStatus(cmd.Context(), model)
		if err != nil {
			return err
		}

		fmt.Printf("notes: %d\n", status.NotesTotal)
		fmt.Printf("embedding model: %s\n", status.EmbeddingModel)
		fmt.Printf("embeddings for current model: %d\n", status.EmbeddingsTotal)
		fmt.Printf("notes with current embeddings: %d\n", status.CurrentForModel)
		fmt.Printf("notes missing embeddings: %d\n", status.MissingForModel)
		fmt.Printf("notes with stale embeddings: %d\n", status.StaleForModel)
		if status.MissingForModel+status.StaleForModel > 0 {
			fmt.Println("run: ai-dev-logger embed --all")
		} else {
			fmt.Println("embedding index is up to date")
		}
		return nil
	},
}
