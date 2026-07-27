package forgemongo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type testFilter struct {
	ID string `bson:"_id"`
}

type otherFilter struct {
	Status string `bson:"status"`
}

type testSort struct {
	CreatedAt int `bson:"createdAt"`
}

func TestCompileQuery(t *testing.T) {
	firstFilter := testFilter{ID: "first"}
	secondFilter := testFilter{ID: "second"}
	sort := testSort{CreatedAt: -1}

	query, err := compileQuery[testFilter, testSort]([]Option{
		WithFilter(firstFilter),
		WithFilter(secondFilter),
		WithSort(sort),
		WithLimit(25),
		WithSkip(50),
	})
	require.NoError(t, err)

	assert.Equal(t, bson.D{{
		Key:   "$or",
		Value: []any{firstFilter, secondFilter},
	}}, query.filter)
	assert.True(t, query.hasFilters)

	findOptions := applyFindOptions(t, query.findOptions())
	require.NotNil(t, findOptions.Limit)
	require.NotNil(t, findOptions.Skip)
	assert.Equal(t, int64(25), *findOptions.Limit)
	assert.Equal(t, int64(50), *findOptions.Skip)
	assert.Equal(t, sort, findOptions.Sort)

	findOneOptions := applyFindOneOptions(t, query.findOneOptions())
	require.NotNil(t, findOneOptions.Skip)
	assert.Equal(t, int64(50), *findOneOptions.Skip)
	assert.Equal(t, sort, findOneOptions.Sort)

	updateOptions := applyUpdateOneOptions(t, query.updateOneOptions())
	assert.Equal(t, sort, updateOptions.Sort)
}

func TestCompileQueryUsesSingleFilterDirectly(t *testing.T) {
	filter := testFilter{ID: "user-1"}

	query, err := compileQuery[testFilter, testSort]([]Option{
		WithFilter(filter),
	})
	require.NoError(t, err)

	assert.Equal(t, filter, query.filter)
}

func TestCompileQueryDefaultsToEmptyDocument(t *testing.T) {
	query, err := compileQuery[testFilter, testSort](nil)
	require.NoError(t, err)

	assert.Equal(t, bson.D{}, query.filter)
	assert.False(t, query.hasFilters)
}

func TestCompileQueryRejectsInvalidOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		opts []Option
	}{
		{
			name: "wrong filter type",
			opts: []Option{WithFilter(otherFilter{Status: "active"})},
		},
		{
			name: "wrong sort type",
			opts: []Option{WithSort(struct{ Name int }{Name: 1})},
		},
		{
			name: "negative limit",
			opts: []Option{WithLimit(-1)},
		},
		{
			name: "negative skip",
			opts: []Option{WithSkip(-1)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileQuery[testFilter, testSort](test.opts)
			require.Error(t, err)
		})
	}
}

func TestCompiledQueryRequiresExplicitScopeForWrites(t *testing.T) {
	query, err := compileQuery[testFilter, testSort](nil)
	require.NoError(t, err)
	require.ErrorIs(t, query.validateWrite(false), errMissingWriteFilter)

	query, err = compileQuery[testFilter, testSort]([]Option{WithAllDocuments()})
	require.NoError(t, err)
	require.NoError(t, query.validateWrite(false))

	query, err = compileQuery[testFilter, testSort]([]Option{
		WithFilter(testFilter{ID: "user-1"}),
	})
	require.NoError(t, err)
	require.NoError(t, query.validateWrite(false))
}

func TestCompiledQueryRejectsReadOptionsForWrites(t *testing.T) {
	for _, test := range []struct {
		name      string
		option    Option
		allowSort bool
	}{
		{
			name:   "limit",
			option: WithLimit(10),
		},
		{
			name:   "skip",
			option: WithSkip(10),
		},
		{
			name:   "sort on bulk write",
			option: WithSort(testSort{CreatedAt: 1}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, err := compileQuery[testFilter, testSort]([]Option{
				WithAllDocuments(),
				test.option,
			})
			require.NoError(t, err)
			require.Error(t, query.validateWrite(test.allowSort))
		})
	}

	query, err := compileQuery[testFilter, testSort]([]Option{
		WithAllDocuments(),
		WithSort(testSort{CreatedAt: 1}),
	})
	require.NoError(t, err)
	require.NoError(t, query.validateWrite(true))
}

func applyFindOptions(t *testing.T, builder *options.FindOptionsBuilder) *options.FindOptions {
	t.Helper()

	result := &options.FindOptions{}
	for _, apply := range builder.List() {
		require.NoError(t, apply(result))
	}

	return result
}

func applyFindOneOptions(t *testing.T, builder *options.FindOneOptionsBuilder) *options.FindOneOptions {
	t.Helper()

	result := &options.FindOneOptions{}
	for _, apply := range builder.List() {
		require.NoError(t, apply(result))
	}

	return result
}

func applyUpdateOneOptions(t *testing.T, builder *options.UpdateOneOptionsBuilder) *options.UpdateOneOptions {
	t.Helper()

	result := &options.UpdateOneOptions{}
	for _, apply := range builder.List() {
		require.NoError(t, apply(result))
	}

	return result
}
