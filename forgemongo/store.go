// Package forgemongo provides a typed, generic store built on the MongoDB Go
// Driver.
package forgemongo

import (
	"context"
	"iter"
)

// Store provides typed CRUD operations for entity T, filter F, sort S, and
// native MongoDB update document P.
type Store[T, F, S, P any] interface {
	// FindOne returns the first document matching the query options.
	FindOne(ctx context.Context, opts ...Option) (*T, error)
	// FindMany returns all documents matching the query options.
	FindMany(ctx context.Context, opts ...Option) ([]*T, error)
	// Stream yields documents matching the query options.
	Stream(ctx context.Context, opts ...Option) iter.Seq2[*T, error]

	// InsertOne inserts a single document.
	InsertOne(ctx context.Context, entity *T) error
	// InsertMany inserts multiple documents.
	InsertMany(ctx context.Context, entities []*T) error

	// PatchOne updates the first document matching the query options.
	PatchOne(ctx context.Context, update P, opts ...Option) error
	// PatchMany updates all documents matching the query options.
	PatchMany(ctx context.Context, update P, opts ...Option) (int64, error)

	// DeleteOne deletes the first document matching the query options.
	DeleteOne(ctx context.Context, opts ...Option) error
	// DeleteMany deletes all documents matching the query options.
	DeleteMany(ctx context.Context, opts ...Option) (int64, error)
}
