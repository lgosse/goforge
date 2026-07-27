package forgemongo

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/lgosse/goforge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type testEntity struct {
	ID     string   `bson:"_id"`
	Status string   `bson:"status"`
	Tags   []string `bson:"tags"`
}

type testSet struct {
	Status string `bson:"status"`
}

type testPush struct {
	Tags string `bson:"tags"`
}

type testPatch struct {
	Set  *testSet  `bson:"$set,omitempty"`
	Push *testPush `bson:"$push,omitempty"`
}

type mockCollection struct {
	findOneFunc    func(context.Context, any, ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
	findFunc       func(context.Context, any, ...options.Lister[options.FindOptions]) (*mongo.Cursor, error)
	insertOneFunc  func(context.Context, any, ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error)
	insertManyFunc func(context.Context, any, ...options.Lister[options.InsertManyOptions]) (*mongo.InsertManyResult, error)
	updateOneFunc  func(context.Context, any, any, ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
	updateManyFunc func(context.Context, any, any, ...options.Lister[options.UpdateManyOptions]) (*mongo.UpdateResult, error)
	deleteOneFunc  func(context.Context, any, ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error)
	deleteManyFunc func(context.Context, any, ...options.Lister[options.DeleteManyOptions]) (*mongo.DeleteResult, error)
}

var _ collection = (*mockCollection)(nil)

func (m *mockCollection) FindOne(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.FindOneOptions],
) *mongo.SingleResult {
	return m.findOneFunc(ctx, filter, opts...)
}

func (m *mockCollection) Find(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.FindOptions],
) (*mongo.Cursor, error) {
	return m.findFunc(ctx, filter, opts...)
}

func (m *mockCollection) InsertOne(
	ctx context.Context,
	document any,
	opts ...options.Lister[options.InsertOneOptions],
) (*mongo.InsertOneResult, error) {
	return m.insertOneFunc(ctx, document, opts...)
}

func (m *mockCollection) InsertMany(
	ctx context.Context,
	documents any,
	opts ...options.Lister[options.InsertManyOptions],
) (*mongo.InsertManyResult, error) {
	return m.insertManyFunc(ctx, documents, opts...)
}

func (m *mockCollection) UpdateOne(
	ctx context.Context,
	filter any,
	update any,
	opts ...options.Lister[options.UpdateOneOptions],
) (*mongo.UpdateResult, error) {
	return m.updateOneFunc(ctx, filter, update, opts...)
}

func (m *mockCollection) UpdateMany(
	ctx context.Context,
	filter any,
	update any,
	opts ...options.Lister[options.UpdateManyOptions],
) (*mongo.UpdateResult, error) {
	return m.updateManyFunc(ctx, filter, update, opts...)
}

func (m *mockCollection) DeleteOne(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.DeleteOneOptions],
) (*mongo.DeleteResult, error) {
	return m.deleteOneFunc(ctx, filter, opts...)
}

func (m *mockCollection) DeleteMany(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.DeleteManyOptions],
) (*mongo.DeleteResult, error) {
	return m.deleteManyFunc(ctx, filter, opts...)
}

func TestFindOne(t *testing.T) {
	filter := testFilter{ID: "user-1"}
	collection := &mockCollection{
		findOneFunc: func(
			_ context.Context,
			gotFilter any,
			_ ...options.Lister[options.FindOneOptions],
		) *mongo.SingleResult {
			assert.Equal(t, filter, gotFilter)
			return mongo.NewSingleResultFromDocument(testEntity{
				ID:     "user-1",
				Status: "active",
			}, nil, nil)
		},
	}
	store := newTestStore(collection)

	entity, err := store.FindOne(t.Context(), WithFilter(filter))
	require.NoError(t, err)
	assert.Equal(t, "user-1", entity.ID)
	assert.Equal(t, "active", entity.Status)
}

func TestFindOneMapsNotFound(t *testing.T) {
	collection := &mockCollection{
		findOneFunc: func(
			context.Context,
			any,
			...options.Lister[options.FindOneOptions],
		) *mongo.SingleResult {
			return mongo.NewSingleResultFromDocument(bson.D{}, mongo.ErrNoDocuments, nil)
		},
	}
	store := newTestStore(collection)

	_, err := store.FindOne(t.Context())

	assertForgeError(t, err, http.StatusNotFound, ErrorCodeNotFound)
	require.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestFindOneMapsNilResultToDatabaseError(t *testing.T) {
	collection := &mockCollection{
		findOneFunc: func(
			context.Context,
			any,
			...options.Lister[options.FindOneOptions],
		) *mongo.SingleResult {
			return nil
		},
	}
	store := newTestStore(collection)

	_, err := store.FindOne(t.Context())

	assertForgeError(t, err, http.StatusInternalServerError, ErrorCodeDatabase)
}

func TestFindMany(t *testing.T) {
	collection := &mockCollection{
		findFunc: func(
			_ context.Context,
			filter any,
			_ ...options.Lister[options.FindOptions],
		) (*mongo.Cursor, error) {
			assert.Equal(t, bson.D{}, filter)
			return newTestCursor(t, []any{
				testEntity{ID: "user-1"},
				testEntity{ID: "user-2"},
			}, nil), nil
		},
	}
	store := newTestStore(collection)

	entities, err := store.FindMany(t.Context(), WithLimit(2))
	require.NoError(t, err)
	require.Len(t, entities, 2)
	assert.Equal(t, "user-1", entities[0].ID)
	assert.Equal(t, "user-2", entities[1].ID)
	assert.NotSame(t, entities[0], entities[1])
}

func TestStream(t *testing.T) {
	collection := &mockCollection{
		findFunc: func(
			context.Context,
			any,
			...options.Lister[options.FindOptions],
		) (*mongo.Cursor, error) {
			return newTestCursor(t, []any{
				testEntity{ID: "user-1"},
				testEntity{ID: "user-2"},
			}, nil), nil
		},
	}
	store := newTestStore(collection)

	var ids []string
	for entity, err := range store.Stream(t.Context()) {
		require.NoError(t, err)
		ids = append(ids, entity.ID)
	}

	assert.Equal(t, []string{"user-1", "user-2"}, ids)
}

func TestStreamMapsCursorError(t *testing.T) {
	collection := &mockCollection{
		findFunc: func(
			context.Context,
			any,
			...options.Lister[options.FindOptions],
		) (*mongo.Cursor, error) {
			return newTestCursor(t, nil, context.DeadlineExceeded), nil
		},
	}
	store := newTestStore(collection)

	var streamErr error
	for _, err := range store.Stream(t.Context()) {
		streamErr = err
	}

	assertForgeError(t, streamErr, http.StatusGatewayTimeout, ErrorCodeTimeout)
	require.ErrorIs(t, streamErr, context.DeadlineExceeded)
}

func TestStreamFreezesOptionsWhenCreated(t *testing.T) {
	var capturedOptions *options.FindOptions
	collection := &mockCollection{
		findFunc: func(
			_ context.Context,
			_ any,
			opts ...options.Lister[options.FindOptions],
		) (*mongo.Cursor, error) {
			require.Len(t, opts, 1)
			capturedOptions = applyFindLister(t, opts[0])
			return newTestCursor(t, nil, nil), nil
		},
	}
	store := newTestStore(collection)
	limit := int64(10)
	option := func(config *QueryConfig) {
		config.Limit = &limit
	}

	sequence := store.Stream(t.Context(), option)
	limit = 20
	for _, err := range sequence {
		require.NoError(t, err)
	}

	require.NotNil(t, capturedOptions)
	require.NotNil(t, capturedOptions.Limit)
	assert.Equal(t, int64(10), *capturedOptions.Limit)
}

func TestInsertOnePassesTypedEntity(t *testing.T) {
	entity := &testEntity{ID: "user-1"}
	collection := &mockCollection{
		insertOneFunc: func(
			_ context.Context,
			document any,
			_ ...options.Lister[options.InsertOneOptions],
		) (*mongo.InsertOneResult, error) {
			assert.Same(t, entity, document)
			return &mongo.InsertOneResult{}, nil
		},
	}
	store := newTestStore(collection)

	require.NoError(t, store.InsertOne(t.Context(), entity))
}

func TestInsertManyRejectsInvalidInput(t *testing.T) {
	store := newTestStore(&mockCollection{})

	assertForgeError(t, store.InsertMany(t.Context(), nil), http.StatusBadRequest, ErrorCodeInvalidArgument)
	assertForgeError(
		t,
		store.InsertMany(t.Context(), []*testEntity{nil}),
		http.StatusBadRequest,
		ErrorCodeInvalidArgument,
	)
}

func TestPatchOnePassesNativeUpdateAndSort(t *testing.T) {
	filter := testFilter{ID: "user-1"}
	sort := testSort{CreatedAt: -1}
	update := testPatch{
		Set:  &testSet{Status: "active"},
		Push: &testPush{Tags: "web_login"},
	}
	collection := &mockCollection{
		updateOneFunc: func(
			_ context.Context,
			gotFilter any,
			gotUpdate any,
			opts ...options.Lister[options.UpdateOneOptions],
		) (*mongo.UpdateResult, error) {
			assert.Equal(t, filter, gotFilter)
			assert.Equal(t, update, gotUpdate)
			require.Len(t, opts, 1)
			updateOptions := applyUpdateOneLister(t, opts[0])
			assert.Equal(t, sort, updateOptions.Sort)
			return &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil
		},
	}
	store := newTestStore(collection)

	err := store.PatchOne(
		t.Context(),
		update,
		WithFilter(filter),
		WithSort(sort),
	)
	require.NoError(t, err)
}

func TestPatchOneMapsNoMatchToNotFound(t *testing.T) {
	collection := &mockCollection{
		updateOneFunc: func(
			context.Context,
			any,
			any,
			...options.Lister[options.UpdateOneOptions],
		) (*mongo.UpdateResult, error) {
			return &mongo.UpdateResult{}, nil
		},
	}
	store := newTestStore(collection)

	err := store.PatchOne(
		t.Context(),
		testPatch{},
		WithFilter(testFilter{ID: "missing"}),
	)

	assertForgeError(t, err, http.StatusNotFound, ErrorCodeNotFound)
	require.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestPatchManyReturnsModifiedCount(t *testing.T) {
	collection := &mockCollection{
		updateManyFunc: func(
			context.Context,
			any,
			any,
			...options.Lister[options.UpdateManyOptions],
		) (*mongo.UpdateResult, error) {
			return &mongo.UpdateResult{MatchedCount: 5, ModifiedCount: 3}, nil
		},
	}
	store := newTestStore(collection)

	count, err := store.PatchMany(
		t.Context(),
		testPatch{},
		WithAllDocuments(),
	)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestDeleteOneMapsNoMatchToNotFound(t *testing.T) {
	collection := &mockCollection{
		deleteOneFunc: func(
			context.Context,
			any,
			...options.Lister[options.DeleteOneOptions],
		) (*mongo.DeleteResult, error) {
			return &mongo.DeleteResult{}, nil
		},
	}
	store := newTestStore(collection)

	err := store.DeleteOne(
		t.Context(),
		WithFilter(testFilter{ID: "missing"}),
	)

	assertForgeError(t, err, http.StatusNotFound, ErrorCodeNotFound)
}

func TestDeleteManyReturnsDeletedCount(t *testing.T) {
	collection := &mockCollection{
		deleteManyFunc: func(
			_ context.Context,
			filter any,
			_ ...options.Lister[options.DeleteManyOptions],
		) (*mongo.DeleteResult, error) {
			assert.Equal(t, bson.D{}, filter)
			return &mongo.DeleteResult{DeletedCount: 4}, nil
		},
	}
	store := newTestStore(collection)

	count, err := store.DeleteMany(t.Context(), WithAllDocuments())
	require.NoError(t, err)
	assert.Equal(t, int64(4), count)
}

func TestWritesRejectImplicitWholeCollectionOperation(t *testing.T) {
	store := newTestStore(&mockCollection{})

	assertForgeError(
		t,
		store.PatchOne(t.Context(), testPatch{}),
		http.StatusBadRequest,
		ErrorCodeInvalidArgument,
	)
	_, err := store.PatchMany(t.Context(), testPatch{})
	assertForgeError(t, err, http.StatusBadRequest, ErrorCodeInvalidArgument)
	assertForgeError(t, store.DeleteOne(t.Context()), http.StatusBadRequest, ErrorCodeInvalidArgument)
	_, err = store.DeleteMany(t.Context())
	assertForgeError(t, err, http.StatusBadRequest, ErrorCodeInvalidArgument)
}

func TestHandleMongoError(t *testing.T) {
	for _, test := range []struct {
		name       string
		cause      error
		statusCode int
		code       string
	}{
		{
			name:       "duplicate key",
			cause:      mongo.CommandError{Code: 11000, Message: "duplicate"},
			statusCode: http.StatusConflict,
			code:       ErrorCodeConflict,
		},
		{
			name:       "deadline",
			cause:      context.DeadlineExceeded,
			statusCode: http.StatusGatewayTimeout,
			code:       ErrorCodeTimeout,
		},
		{
			name:       "canceled",
			cause:      context.Canceled,
			statusCode: http.StatusRequestTimeout,
			code:       ErrorCodeCanceled,
		},
		{
			name:       "database failure",
			cause:      errors.New("connection failed"),
			statusCode: http.StatusInternalServerError,
			code:       ErrorCodeDatabase,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := handleMongoError(test.cause)

			assertForgeError(t, err, test.statusCode, test.code)
			var forgeErr *goforge.Error
			require.ErrorAs(t, err, &forgeErr)
			assert.Equal(t, test.cause, forgeErr.Cause)
		})
	}
}

func TestNewStoreWithNilDatabaseReturnsConfigurationError(t *testing.T) {
	store := NewStore[testEntity, testFilter, testSort, testPatch](nil, "users")

	_, err := store.FindMany(t.Context())
	assertForgeError(t, err, http.StatusInternalServerError, ErrorCodeDatabase)
	require.ErrorIs(t, err, errNilDatabase)
}

func newTestStore(coll collection) *mongoStore[testEntity, testFilter, testSort, testPatch] {
	return &mongoStore[testEntity, testFilter, testSort, testPatch]{coll: coll}
}

func newTestCursor(t *testing.T, documents []any, cursorErr error) *mongo.Cursor {
	t.Helper()

	cursor, err := mongo.NewCursorFromDocuments(documents, cursorErr, nil)
	require.NoError(t, err)

	return cursor
}

func applyUpdateOneLister(
	t *testing.T,
	lister options.Lister[options.UpdateOneOptions],
) *options.UpdateOneOptions {
	t.Helper()

	result := &options.UpdateOneOptions{}
	for _, apply := range lister.List() {
		require.NoError(t, apply(result))
	}

	return result
}

func applyFindLister(
	t *testing.T,
	lister options.Lister[options.FindOptions],
) *options.FindOptions {
	t.Helper()

	result := &options.FindOptions{}
	for _, apply := range lister.List() {
		require.NoError(t, apply(result))
	}

	return result
}

func assertForgeError(t *testing.T, err error, statusCode int, code string) {
	t.Helper()

	var forgeErr *goforge.Error
	require.ErrorAs(t, err, &forgeErr)
	assert.Equal(t, statusCode, forgeErr.HTTPStatus)
	assert.Equal(t, code, forgeErr.Code)
}
