package forgemongo

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
)

// CallCounts contains a snapshot of Mock method invocation counts.
type CallCounts struct {
	// FindOne is the number of FindOne calls.
	FindOne uint64
	// FindMany is the number of FindMany calls.
	FindMany uint64
	// Stream is the number of Stream calls.
	Stream uint64
	// InsertOne is the number of InsertOne calls.
	InsertOne uint64
	// InsertMany is the number of InsertMany calls.
	InsertMany uint64
	// PatchOne is the number of PatchOne calls.
	PatchOne uint64
	// PatchMany is the number of PatchMany calls.
	PatchMany uint64
	// DeleteOne is the number of DeleteOne calls.
	DeleteOne uint64
	// DeleteMany is the number of DeleteMany calls.
	DeleteMany uint64
}

// Mock is a configurable in-memory implementation of Store.
//
// Configure its function fields before using it in concurrent tests.
type Mock[T, F, S, P any] struct {
	// FindOneFunc handles FindOne calls.
	FindOneFunc func(context.Context, ...Option) (*T, error)
	// FindManyFunc handles FindMany calls.
	FindManyFunc func(context.Context, ...Option) ([]*T, error)
	// StreamFunc handles Stream calls.
	StreamFunc func(context.Context, ...Option) iter.Seq2[*T, error]
	// InsertOneFunc handles InsertOne calls.
	InsertOneFunc func(context.Context, *T) error
	// InsertManyFunc handles InsertMany calls.
	InsertManyFunc func(context.Context, []*T) error
	// PatchOneFunc handles PatchOne calls.
	PatchOneFunc func(context.Context, P, ...Option) error
	// PatchManyFunc handles PatchMany calls.
	PatchManyFunc func(context.Context, P, ...Option) (int64, error)
	// DeleteOneFunc handles DeleteOne calls.
	DeleteOneFunc func(context.Context, ...Option) error
	// DeleteManyFunc handles DeleteMany calls.
	DeleteManyFunc func(context.Context, ...Option) (int64, error)

	findOneCalls    atomic.Uint64
	findManyCalls   atomic.Uint64
	streamCalls     atomic.Uint64
	insertOneCalls  atomic.Uint64
	insertManyCalls atomic.Uint64
	patchOneCalls   atomic.Uint64
	patchManyCalls  atomic.Uint64
	deleteOneCalls  atomic.Uint64
	deleteManyCalls atomic.Uint64
}

// NewMock creates a Store mock whose behavior is configured through function fields.
func NewMock[T, F, S, P any]() *Mock[T, F, S, P] {
	return &Mock[T, F, S, P]{}
}

// Calls returns a concurrency-safe snapshot of method invocation counts.
func (m *Mock[T, F, S, P]) Calls() CallCounts {
	if m == nil {
		return CallCounts{}
	}

	return CallCounts{
		FindOne:    m.findOneCalls.Load(),
		FindMany:   m.findManyCalls.Load(),
		Stream:     m.streamCalls.Load(),
		InsertOne:  m.insertOneCalls.Load(),
		InsertMany: m.insertManyCalls.Load(),
		PatchOne:   m.patchOneCalls.Load(),
		PatchMany:  m.patchManyCalls.Load(),
		DeleteOne:  m.deleteOneCalls.Load(),
		DeleteMany: m.deleteManyCalls.Load(),
	}
}

// FindOne delegates to FindOneFunc.
func (m *Mock[T, F, S, P]) FindOne(ctx context.Context, opts ...Option) (*T, error) {
	if m != nil {
		m.findOneCalls.Add(1)
	}
	if m != nil && m.FindOneFunc != nil {
		return m.FindOneFunc(ctx, opts...)
	}

	return nil, unexpectedMockCall("FindOne")
}

// FindMany delegates to FindManyFunc.
func (m *Mock[T, F, S, P]) FindMany(ctx context.Context, opts ...Option) ([]*T, error) {
	if m != nil {
		m.findManyCalls.Add(1)
	}
	if m != nil && m.FindManyFunc != nil {
		return m.FindManyFunc(ctx, opts...)
	}

	return nil, unexpectedMockCall("FindMany")
}

// Stream delegates to StreamFunc.
func (m *Mock[T, F, S, P]) Stream(ctx context.Context, opts ...Option) iter.Seq2[*T, error] {
	if m != nil {
		m.streamCalls.Add(1)
	}
	if m != nil && m.StreamFunc != nil {
		return m.StreamFunc(ctx, opts...)
	}

	return func(yield func(*T, error) bool) {
		yield(nil, unexpectedMockCall("Stream"))
	}
}

// InsertOne delegates to InsertOneFunc.
func (m *Mock[T, F, S, P]) InsertOne(ctx context.Context, entity *T) error {
	if m != nil {
		m.insertOneCalls.Add(1)
	}
	if m != nil && m.InsertOneFunc != nil {
		return m.InsertOneFunc(ctx, entity)
	}

	return unexpectedMockCall("InsertOne")
}

// InsertMany delegates to InsertManyFunc.
func (m *Mock[T, F, S, P]) InsertMany(ctx context.Context, entities []*T) error {
	if m != nil {
		m.insertManyCalls.Add(1)
	}
	if m != nil && m.InsertManyFunc != nil {
		return m.InsertManyFunc(ctx, entities)
	}

	return unexpectedMockCall("InsertMany")
}

// PatchOne delegates to PatchOneFunc.
func (m *Mock[T, F, S, P]) PatchOne(ctx context.Context, update P, opts ...Option) error {
	if m != nil {
		m.patchOneCalls.Add(1)
	}
	if m != nil && m.PatchOneFunc != nil {
		return m.PatchOneFunc(ctx, update, opts...)
	}

	return unexpectedMockCall("PatchOne")
}

// PatchMany delegates to PatchManyFunc.
func (m *Mock[T, F, S, P]) PatchMany(ctx context.Context, update P, opts ...Option) (int64, error) {
	if m != nil {
		m.patchManyCalls.Add(1)
	}
	if m != nil && m.PatchManyFunc != nil {
		return m.PatchManyFunc(ctx, update, opts...)
	}

	return 0, unexpectedMockCall("PatchMany")
}

// DeleteOne delegates to DeleteOneFunc.
func (m *Mock[T, F, S, P]) DeleteOne(ctx context.Context, opts ...Option) error {
	if m != nil {
		m.deleteOneCalls.Add(1)
	}
	if m != nil && m.DeleteOneFunc != nil {
		return m.DeleteOneFunc(ctx, opts...)
	}

	return unexpectedMockCall("DeleteOne")
}

// DeleteMany delegates to DeleteManyFunc.
func (m *Mock[T, F, S, P]) DeleteMany(ctx context.Context, opts ...Option) (int64, error) {
	if m != nil {
		m.deleteManyCalls.Add(1)
	}
	if m != nil && m.DeleteManyFunc != nil {
		return m.DeleteManyFunc(ctx, opts...)
	}

	return 0, unexpectedMockCall("DeleteMany")
}

func unexpectedMockCall(method string) error {
	return fmt.Errorf("forgemongo mock: %sFunc is not configured", method)
}
