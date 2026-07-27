package forgemongo_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lgosse/goforge/forgemongo"
)

func TestMockImplementsStore(t *testing.T) {
	t.Parallel()

	var store forgemongo.Store[apiEntity, apiFilter, apiSort, apiPatch] = forgemongo.NewMock[apiEntity, apiFilter, apiSort, apiPatch]()

	require.NotNil(t, store)
}

func TestMockDelegatesConfiguredCalls(t *testing.T) {
	t.Parallel()

	want := &apiEntity{ID: "entity-id"}
	mock := forgemongo.NewMock[apiEntity, apiFilter, apiSort, apiPatch]()
	mock.FindOneFunc = func(_ context.Context, opts ...forgemongo.Option) (*apiEntity, error) {
		assert.Len(t, opts, 1)
		return want, nil
	}

	got, err := mock.FindOne(context.Background(), forgemongo.WithLimit(1))

	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestMockReturnsErrorForUnconfiguredCall(t *testing.T) {
	t.Parallel()

	mock := forgemongo.NewMock[apiEntity, apiFilter, apiSort, apiPatch]()

	_, err := mock.FindOne(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "FindOneFunc is not configured")
	assert.Equal(t, uint64(1), mock.Calls().FindOne)
}

func TestMockStreamYieldsErrorWhenUnconfigured(t *testing.T) {
	t.Parallel()

	mock := forgemongo.NewMock[apiEntity, apiFilter, apiSort, apiPatch]()
	var received error

	for _, err := range mock.Stream(context.Background()) {
		received = err
	}

	require.Error(t, received)
	assert.False(t, errors.Is(received, context.Canceled))
	assert.Contains(t, received.Error(), "StreamFunc is not configured")
	assert.Equal(t, uint64(1), mock.Calls().Stream)
}

func TestMockCountsConcurrentCalls(t *testing.T) {
	t.Parallel()

	const callCount = 100

	mock := forgemongo.NewMock[apiEntity, apiFilter, apiSort, apiPatch]()
	mock.FindOneFunc = func(context.Context, ...forgemongo.Option) (*apiEntity, error) {
		return nil, nil
	}

	var wg sync.WaitGroup
	for range callCount {
		// #Exercise the mock concurrently to verify atomic call accounting.
		wg.Go(func() {
			_, _ = mock.FindOne(context.Background())
		})
	}
	wg.Wait()

	assert.Equal(t, uint64(callCount), mock.Calls().FindOne)
}
