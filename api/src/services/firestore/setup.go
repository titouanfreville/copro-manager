package firestore

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"
)

// Config holds the Firestore client configuration.
type Config struct {
	ProjectID string `yaml:"project_id"`
	Database  string `yaml:"database"`
}

// NewClient returns a new Firestore client for the configured project + database.
func NewClient(conf Config) (*fs.Client, error) {
	ctx := context.Background()

	if conf.Database == "" || conf.Database == "(default)" {
		return fs.NewClient(ctx, conf.ProjectID)
	}

	client, err := fs.NewClientWithDatabase(ctx, conf.ProjectID, conf.Database)
	if err != nil {
		return nil, fmt.Errorf("firestore: connect to %s/%s: %w", conf.ProjectID, conf.Database, err)
	}

	return client, nil
}
